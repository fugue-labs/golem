package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ReplayEventKind identifies the type of recorded event.
type ReplayEventKind string

const (
	EventUserInput   ReplayEventKind = "user_input"
	EventTextDelta   ReplayEventKind = "text_delta"
	EventThinkDelta  ReplayEventKind = "thinking_delta"
	EventToolCall    ReplayEventKind = "tool_call"
	EventToolResult  ReplayEventKind = "tool_result"
	EventAgentDone   ReplayEventKind = "agent_done"
	EventSystem      ReplayEventKind = "system"
	EventError       ReplayEventKind = "error"
)

// ReplayEvent is a single recorded event in a trace.
type ReplayEvent struct {
	Kind     ReplayEventKind `json:"kind"`
	OffsetMs int64           `json:"offset_ms"` // milliseconds since trace start
	Data     json.RawMessage `json:"data"`
}

// Type-specific event payloads.

type UserInputData struct {
	Text string `json:"text"`
}

type TextDeltaData struct {
	Text string `json:"text"`
}

type ThinkDeltaData struct {
	Text string `json:"text"`
}

type ToolCallData struct {
	CallID  string `json:"call_id"`
	Name    string `json:"name"`
	Args    string `json:"args"`
	RawArgs string `json:"raw_args"`
}

type ToolResultData struct {
	CallID string `json:"call_id"`
	Name   string `json:"name"`
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

type AgentDoneData struct {
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	ToolCalls    int    `json:"tool_calls"`
	Error        string `json:"error,omitempty"`
}

type SystemEventData struct {
	Text string `json:"text"`
}

type ErrorEventData struct {
	Text string `json:"text"`
}

// ReplayTrace is a complete recorded session trace.
type ReplayTrace struct {
	Version   int           `json:"version"`
	StartTime time.Time     `json:"start_time"`
	Model     string        `json:"model"`
	Provider  string        `json:"provider"`
	WorkDir   string        `json:"work_dir"`
	Events    []ReplayEvent `json:"events"`

	mu sync.Mutex `json:"-"`
}

// TraceInfo is summary metadata about a saved trace file.
type TraceInfo struct {
	Filename  string
	Timestamp time.Time
	Model     string
	Provider  string
	Events    int
}

const traceVersion = 1

// NewTrace creates a new replay trace ready for recording.
func NewTrace(model, provider, workDir string) *ReplayTrace {
	return &ReplayTrace{
		Version:   traceVersion,
		StartTime: time.Now(),
		Model:     model,
		Provider:  provider,
		WorkDir:   workDir,
	}
}

// Record appends an event to the trace with the current time offset.
func (t *ReplayTrace) Record(kind ReplayEventKind, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Events = append(t.Events, ReplayEvent{
		Kind:     kind,
		OffsetMs: time.Since(t.StartTime).Milliseconds(),
		Data:     data,
	})
}

// Save writes the trace to the session directory as a .replay.json file.
func (t *ReplayTrace) Save(workDir string) error {
	dir, err := SessionDir(workDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	t.mu.Lock()
	raw, err := json.Marshal(t)
	t.mu.Unlock()
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s.replay.json", t.StartTime.Format("2006-01-02T15-04-05"))
	return os.WriteFile(filepath.Join(dir, filename), raw, 0644)
}

// LoadTrace loads a specific trace file by path.
func LoadTrace(path string) (*ReplayTrace, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var trace ReplayTrace
	if err := json.Unmarshal(raw, &trace); err != nil {
		return nil, err
	}
	return &trace, nil
}

// LoadLatestTrace loads the most recent replay trace for the given working directory.
func LoadLatestTrace(workDir string) (*ReplayTrace, error) {
	dir, err := SessionDir(workDir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var traceFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".replay.json") {
			traceFiles = append(traceFiles, e.Name())
		}
	}
	if len(traceFiles) == 0 {
		return nil, nil
	}
	sort.Strings(traceFiles)
	latest := traceFiles[len(traceFiles)-1]

	return LoadTrace(filepath.Join(dir, latest))
}

// ListTraces returns metadata about available trace files for a project.
func ListTraces(workDir string) ([]TraceInfo, error) {
	dir, err := SessionDir(workDir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var traces []TraceInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".replay.json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var t ReplayTrace
		if err := json.Unmarshal(raw, &t); err != nil {
			continue
		}
		traces = append(traces, TraceInfo{
			Filename:  e.Name(),
			Timestamp: t.StartTime,
			Model:     t.Model,
			Provider:  t.Provider,
			Events:    len(t.Events),
		})
	}
	sort.Slice(traces, func(i, j int) bool {
		return traces[i].Timestamp.Before(traces[j].Timestamp)
	})
	return traces, nil
}

// DecodeEvent unmarshals a ReplayEvent's data into the appropriate typed struct.
func DecodeEvent[T any](event ReplayEvent) (T, error) {
	var v T
	err := json.Unmarshal(event.Data, &v)
	return v, err
}
