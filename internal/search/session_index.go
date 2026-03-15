package search

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// SessionResult represents a search hit with session metadata.
type SessionResult struct {
	SessionFile string    // path to session JSON file
	ProjectDir  string    // original working directory of the session
	Timestamp   time.Time // when the session was saved
	Prompt      string    // the user's prompt for this session
	Score       float64   // BM25 relevance score
	Snippet     string    // text snippet around the match
}

// sessionData mirrors agent.SessionData for unmarshaling.
type sessionData struct {
	Messages   json.RawMessage `json:"messages"`
	Transcript json.RawMessage `json:"transcript,omitempty"`
	WorkDir    string          `json:"work_dir"`
	Timestamp  time.Time       `json:"timestamp"`
	Prompt     string          `json:"prompt,omitempty"`
	Model      string          `json:"model"`
}

// messageContent is a minimal struct for extracting text from legacy messages.
type messageContent struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type transcriptEntry struct {
	Kind     int
	Content  string
	ToolName string
	ToolArgs string
	RawArgs  string
}

type sessionMeta struct {
	file      string
	workDir   string
	timestamp time.Time
	prompt    string
	session   *sessionData
}

// SearchSessions searches across all saved sessions for the given query.
// If projectDir is non-empty, only sessions from that project are searched.
// Returns up to maxResults results sorted by relevance with richer transcript-aware snippets when available.
func SearchSessions(query string, projectDir string, maxResults int) ([]SessionResult, error) {
	sessionsRoot, err := sessionsBaseDir()
	if err != nil {
		return nil, fmt.Errorf("finding sessions dir: %w", err)
	}

	idx := NewIndex()
	metadata := make(map[string]sessionMeta)

	entries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectPath := filepath.Join(sessionsRoot, entry.Name())
		sessions, err := os.ReadDir(projectPath)
		if err != nil {
			continue
		}

		for _, sf := range sessions {
			if sf.IsDir() || !strings.HasSuffix(sf.Name(), ".json") {
				continue
			}
			sessionFile := filepath.Join(projectPath, sf.Name())
			sd, err := loadSessionData(sessionFile)
			if err != nil {
				continue
			}
			if projectDir != "" && sd.WorkDir != projectDir {
				continue
			}

			text := extractSessionText(sd)
			if text == "" {
				continue
			}

			idx.Add(sessionFile, text)
			metadata[sessionFile] = sessionMeta{
				file:      sessionFile,
				workDir:   sd.WorkDir,
				timestamp: sd.Timestamp,
				prompt:    sd.Prompt,
				session:   sd,
			}
		}
	}

	results := idx.Search(query, maxResults)
	out := make([]SessionResult, 0, len(results))
	for _, r := range results {
		meta, ok := metadata[r.DocID]
		if !ok {
			continue
		}
		out = append(out, SessionResult{
			SessionFile: meta.file,
			ProjectDir:  meta.workDir,
			Timestamp:   meta.timestamp,
			Prompt:      meta.prompt,
			Score:       r.Score,
			Snippet:     extractSessionSnippet(meta.session, r.Doc.Text, query, 280),
		})
	}
	return out, nil
}

// ListProjects returns all project directories that have saved sessions.
func ListProjects() ([]string, error) {
	sessionsRoot, err := sessionsBaseDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var projects []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectPath := filepath.Join(sessionsRoot, entry.Name())
		sessions, err := os.ReadDir(projectPath)
		if err != nil {
			continue
		}
		var jsonFiles []string
		for _, sf := range sessions {
			if !sf.IsDir() && strings.HasSuffix(sf.Name(), ".json") {
				jsonFiles = append(jsonFiles, filepath.Join(projectPath, sf.Name()))
			}
		}
		if len(jsonFiles) == 0 {
			continue
		}
		sort.Strings(jsonFiles)
		sd, err := loadSessionData(jsonFiles[len(jsonFiles)-1])
		if err != nil {
			continue
		}
		if sd.WorkDir != "" {
			projects = append(projects, sd.WorkDir)
		}
	}
	return projects, nil
}

func sessionsBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".golem", "sessions"), nil
}

func loadSessionData(path string) (*sessionData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sd sessionData
	if err := json.Unmarshal(raw, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}

// extractSessionText extracts searchable text from a session.
// Includes the prompt and all user/assistant/tool text that can be recovered from saved state.
func extractSessionText(sd *sessionData) string {
	var lines []string
	appendSearchLine(&lines, sd.Prompt)

	if structured := extractStructuredMessageText(sd.Messages); structured != "" {
		for _, line := range strings.Split(structured, "\n") {
			appendSearchLine(&lines, line)
		}
	} else if legacy := extractLegacyMessageText(sd.Messages); legacy != "" {
		for _, line := range strings.Split(legacy, "\n") {
			appendSearchLine(&lines, line)
		}
	}

	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func extractStructuredMessageText(raw json.RawMessage) string {
	messages, err := core.UnmarshalMessages(raw)
	if err != nil {
		return ""
	}

	var lines []string
	for _, msg := range messages {
		switch m := msg.(type) {
		case core.ModelRequest:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.UserPromptPart:
					appendSearchLine(&lines, p.Content)
				case core.RetryPromptPart:
					appendSearchLine(&lines, p.Content)
				case core.ToolReturnPart:
					appendSearchLine(&lines, p.ToolName)
					appendSearchLine(&lines, stringifySearchValue(p.Content))
				}
			}
		case core.ModelResponse:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.TextPart:
					appendSearchLine(&lines, p.Content)
				case core.ThinkingPart:
					appendSearchLine(&lines, p.Content)
				case core.ToolCallPart:
					appendSearchLine(&lines, p.ToolName)
					appendSearchLine(&lines, p.ArgsJSON)
				}
			}
		}
	}

	return strings.Join(lines, "\n")
}

func extractLegacyMessageText(raw json.RawMessage) string {
	var messages []messageContent
	if err := json.Unmarshal(raw, &messages); err != nil {
		return ""
	}

	var lines []string
	for _, msg := range messages {
		switch content := msg.Content.(type) {
		case string:
			appendSearchLine(&lines, content)
		case []any:
			for _, block := range content {
				if m, ok := block.(map[string]any); ok {
					if text, ok := m["text"].(string); ok {
						appendSearchLine(&lines, text)
					}
					if text, ok := m["Text"].(string); ok {
						appendSearchLine(&lines, text)
					}
				}
			}
		}
	}

	return strings.Join(lines, "\n")
}

func stringifySearchValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.RawMessage:
		return normalizeSearchText(string(x))
	case []byte:
		return normalizeSearchText(string(x))
	default:
		raw, err := json.Marshal(x)
		if err != nil {
			return normalizeSearchText(fmt.Sprint(x))
		}
		return normalizeSearchText(string(raw))
	}
}

func appendSearchLine(lines *[]string, text string) {
	text = normalizeSearchText(text)
	if text == "" {
		return
	}
	*lines = append(*lines, text)
}

func normalizeSearchText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func extractSessionSnippet(sd *sessionData, fallbackText, query string, maxLen int) string {
	if sd != nil {
		if snippet := extractTranscriptSnippet(sd.Transcript, query, maxLen); snippet != "" {
			return snippet
		}
	}
	return extractSnippet(fallbackText, query, maxLen)
}

func extractTranscriptSnippet(raw json.RawMessage, query string, maxLen int) string {
	lines := transcriptDisplayLines(raw)
	if len(lines) == 0 {
		return ""
	}
	idx := findMatchingLine(lines, query)
	if idx < 0 {
		return ""
	}

	window := snippetLineWindow(lines, idx)
	return joinSnippetLines(window, maxLen)
}

func transcriptDisplayLines(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	entries, err := decodeTranscriptEntries(raw)
	if err != nil {
		return nil
	}

	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		if line := formatTranscriptLine(entry); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func decodeTranscriptEntries(raw json.RawMessage) ([]transcriptEntry, error) {
	var wire []map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, err
	}

	entries := make([]transcriptEntry, 0, len(wire))
	for _, item := range wire {
		entry := transcriptEntry{
			Kind:     decodeTranscriptKind(item),
			Content:  firstString(item, "content", "Content"),
			ToolName: firstString(item, "tool_name", "toolName", "ToolName"),
			ToolArgs: firstString(item, "tool_args", "toolArgs", "ToolArgs"),
			RawArgs:  firstString(item, "raw_args", "rawArgs", "RawArgs"),
		}
		if entry.Content == "" {
			entry.Content = decodeTranscriptContent(item)
		}
		if entry.ToolName == "" && entry.Kind == 2 {
			entry.ToolName = firstString(item, "name", "Name")
		}
		if entry.ToolArgs == "" {
			entry.ToolArgs = decodeTranscriptToolArgs(item)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func decodeTranscriptKind(item map[string]any) int {
	for _, key := range []string{"kind", "Kind"} {
		if v, ok := item[key]; ok {
			switch x := v.(type) {
			case float64:
				return int(x)
			case int:
				return x
			case string:
				switch strings.ToLower(strings.TrimSpace(x)) {
				case "user", "kinduser":
					return 0
				case "assistant", "kindassistant":
					return 1
				case "toolcall", "tool_call", "kindtoolcall":
					return 2
				case "toolresult", "tool_result", "kindtoolresult":
					return 3
				case "thinking", "kindthinking":
					return 4
				case "error", "kinderror":
					return 5
				case "system", "kindsystem":
					return 6
				}
			}
		}
	}
	return 0
}

func decodeTranscriptContent(item map[string]any) string {
	for _, key := range []string{"result", "Result", "text", "Text"} {
		if text := stringifyField(item[key]); text != "" {
			return text
		}
	}
	return ""
}

func decodeTranscriptToolArgs(item map[string]any) string {
	for _, key := range []string{"args", "Args"} {
		if text := stringifyField(item[key]); text != "" {
			return text
		}
	}
	return ""
}

func firstString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := stringifyField(item[key]); text != "" {
			return text
		}
	}
	return ""
}

func stringifyField(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return normalizeSearchText(x)
	case []byte:
		return normalizeSearchText(string(x))
	case json.RawMessage:
		return normalizeSearchText(string(x))
	default:
		return stringifySearchValue(x)
	}
}

func snippetLineWindow(lines []string, idx int) []string {
	if idx < 0 || idx >= len(lines) {
		return nil
	}
	start := idx - 1
	if start < 0 {
		start = 0
	}
	end := idx + 2
	if end > len(lines) {
		end = len(lines)
	}
	return lines[start:end]
}

func joinSnippetLines(lines []string, maxLen int) string {
	if len(lines) == 0 || maxLen <= 0 {
		return ""
	}
	selected := make([]string, 0, len(lines))
	remaining := maxLen
	for _, line := range lines {
		if remaining <= 0 {
			break
		}
		budget := remaining
		if budget > 160 {
			budget = 160
		}
		line = truncateRunes(line, budget)
		if line == "" {
			continue
		}
		selected = append(selected, line)
		remaining -= len([]rune(line)) + 1
	}
	return strings.Join(selected, "\n")
}

func formatTranscriptLine(entry transcriptEntry) string {
	content := normalizeSearchText(entry.Content)
	switch entry.Kind {
	case 0:
		if content == "" {
			return ""
		}
		return "User: " + content
	case 1:
		if content == "" {
			return ""
		}
		return "Assistant: " + content
	case 2:
		parts := make([]string, 0, 4)
		if entry.ToolName != "" {
			parts = append(parts, entry.ToolName)
		}
		if entry.ToolArgs != "" {
			parts = append(parts, normalizeSearchText(entry.ToolArgs))
		}
		if entry.RawArgs != "" && normalizeSearchText(entry.RawArgs) != normalizeSearchText(entry.ToolArgs) {
			parts = append(parts, normalizeSearchText(entry.RawArgs))
		}
		if content != "" {
			parts = append(parts, content)
		}
		if len(parts) == 0 {
			return ""
		}
		return "Tool: " + strings.Join(parts, " — ")
	case 3:
		if content == "" {
			return ""
		}
		if entry.ToolName != "" {
			return fmt.Sprintf("Tool result (%s): %s", entry.ToolName, content)
		}
		return "Tool result: " + content
	case 4:
		if content == "" {
			return ""
		}
		return "Thinking: " + content
	case 5:
		if content == "" {
			return ""
		}
		return "Error: " + content
	case 6:
		if content == "" {
			return ""
		}
		return "System: " + content
	default:
		return content
	}
}

func findMatchingLine(lines []string, query string) int {
	terms := tokenize(query)
	if len(terms) == 0 {
		return -1
	}
	for i, line := range lines {
		lower := strings.ToLower(line)
		for _, term := range terms {
			if strings.Contains(lower, term) {
				return i
			}
		}
	}
	return -1
}

func truncateRunes(text string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	if maxLen == 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

// extractSnippet returns a text snippet around the first occurrence of any query term.
func extractSnippet(text, query string, maxLen int) string {
	lower := strings.ToLower(text)
	queryTerms := tokenize(query)

	bestPos := -1
	for _, term := range queryTerms {
		pos := strings.Index(lower, term)
		if pos >= 0 && (bestPos < 0 || pos < bestPos) {
			bestPos = pos
		}
	}

	if bestPos < 0 {
		if len(text) > maxLen {
			return text[:maxLen] + "…"
		}
		return text
	}

	start := bestPos - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
	}

	snippet := text[start:end]
	if start > 0 {
		if idx := strings.IndexByte(snippet, ' '); idx >= 0 && idx < 30 {
			snippet = snippet[idx+1:]
		}
		snippet = "…" + snippet
	}
	if end < len(text) {
		if idx := strings.LastIndexByte(snippet, ' '); idx >= 0 && idx > len(snippet)-30 {
			snippet = snippet[:idx]
		}
		snippet += "…"
	}

	snippet = strings.Join(strings.Fields(snippet), " ")
	return snippet
}
