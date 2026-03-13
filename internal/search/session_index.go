package search

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	Messages  json.RawMessage `json:"messages"`
	WorkDir   string          `json:"work_dir"`
	Timestamp time.Time       `json:"timestamp"`
	Prompt    string          `json:"prompt,omitempty"`
	Model     string          `json:"model"`
}

// messageContent is a minimal struct for extracting text from messages.
type messageContent struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// SearchSessions searches across all saved sessions for the given query.
// If projectDir is non-empty, only sessions from that project are searched.
// Returns up to maxResults results sorted by relevance.
func SearchSessions(query string, projectDir string, maxResults int) ([]SessionResult, error) {
	sessionsRoot, err := sessionsBaseDir()
	if err != nil {
		return nil, fmt.Errorf("finding sessions dir: %w", err)
	}

	idx := NewIndex()
	var metadata []sessionMeta

	// Walk all project directories.
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

			// Filter by project if specified.
			if projectDir != "" && sd.WorkDir != projectDir {
				continue
			}

			text := extractSessionText(sd)
			if text == "" {
				continue
			}

			docID := sessionFile
			idx.Add(docID, text)
			metadata = append(metadata, sessionMeta{
				file:      sessionFile,
				workDir:   sd.WorkDir,
				timestamp: sd.Timestamp,
				prompt:    sd.Prompt,
			})
		}
	}

	results := idx.Search(query, maxResults)

	var out []SessionResult
	for _, r := range results {
		// Find metadata by matching doc ID to session file.
		for _, meta := range metadata {
			if meta.file == r.DocID {
				out = append(out, SessionResult{
					SessionFile: meta.file,
					ProjectDir:  meta.workDir,
					Timestamp:   meta.timestamp,
					Prompt:      meta.prompt,
					Score:       r.Score,
					Snippet:     extractSnippet(r.Doc.Text, query, 200),
				})
				break
			}
		}
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
		// Find the work_dir from the most recent session.
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

type sessionMeta struct {
	file      string
	workDir   string
	timestamp time.Time
	prompt    string
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
// Includes the prompt and all user/assistant message content.
func extractSessionText(sd *sessionData) string {
	var b strings.Builder

	// Include the prompt.
	if sd.Prompt != "" {
		b.WriteString(sd.Prompt)
		b.WriteString("\n")
	}

	// Parse messages to extract text content.
	var messages []messageContent
	if err := json.Unmarshal(sd.Messages, &messages); err != nil {
		// Try as a different format — some sessions wrap content differently.
		return b.String()
	}

	for _, msg := range messages {
		switch content := msg.Content.(type) {
		case string:
			b.WriteString(content)
			b.WriteString("\n")
		case []any:
			// Content blocks (e.g., [{type: "text", text: "..."}]).
			for _, block := range content {
				if m, ok := block.(map[string]any); ok {
					if text, ok := m["text"].(string); ok {
						b.WriteString(text)
						b.WriteString("\n")
					}
				}
			}
		}
	}

	return b.String()
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
		// No direct substring match; return the beginning.
		if len(text) > maxLen {
			return text[:maxLen] + "…"
		}
		return text
	}

	// Center the snippet around the match.
	start := bestPos - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
	}

	snippet := text[start:end]
	// Clean up: trim to word boundaries.
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

	// Collapse whitespace for display.
	snippet = strings.Join(strings.Fields(snippet), " ")
	return snippet
}
