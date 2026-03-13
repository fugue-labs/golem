package acp

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestTransportRoundTrip(t *testing.T) {
	// Write a request and read it back.
	var buf bytes.Buffer
	tr := NewTransport(strings.NewReader(""), &buf)

	err := tr.SendNotification(Notification{
		Method: "session/update",
		Params: map[string]string{"sessionId": "sess_1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	line := strings.TrimSpace(buf.String())
	var notif Notification
	if err := json.Unmarshal([]byte(line), &notif); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if notif.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %q", notif.JSONRPC)
	}
	if notif.Method != "session/update" {
		t.Errorf("expected method session/update, got %q", notif.Method)
	}
}

func TestTransportReadRequest(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}` + "\n"
	tr := NewTransport(strings.NewReader(input), io.Discard)

	req, err := tr.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "initialize" {
		t.Errorf("expected method initialize, got %q", req.Method)
	}
}

func TestTransportReadRequestEOF(t *testing.T) {
	tr := NewTransport(strings.NewReader(""), io.Discard)
	_, err := tr.ReadRequest()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestInitializeHandshake(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientInfo":{"name":"test-editor","version":"1.0"}}}` + "\n"

	var out bytes.Buffer
	srv := NewServer(strings.NewReader(input), &out)
	srv.Run()

	var resp Response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var result InitializeResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatal(err)
	}

	if result.ProtocolVersion != 1 {
		t.Errorf("expected protocol version 1, got %d", result.ProtocolVersion)
	}
	if result.AgentInfo.Name != "golem" {
		t.Errorf("expected agent name golem, got %q", result.AgentInfo.Name)
	}
	if !result.AgentCapabilities.Session {
		t.Error("expected session capability to be true")
	}
}

func TestSessionNew(t *testing.T) {
	lines := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp","mcpServers":[]}}`,
	}
	input := strings.Join(lines, "\n") + "\n"

	var out bytes.Buffer
	srv := NewServer(strings.NewReader(input), &out)
	srv.Run()

	// Parse both responses.
	responses := parseResponses(t, out.Bytes())
	if len(responses) < 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}

	// Second response should be session/new result.
	resp := responses[1]
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var result NewSessionResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatal(err)
	}
	if result.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
	if !strings.HasPrefix(result.SessionID, "sess_") {
		t.Errorf("expected session ID prefix sess_, got %q", result.SessionID)
	}
}

func TestSessionNewWithoutInitialize(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/tmp","mcpServers":[]}}` + "\n"

	var out bytes.Buffer
	srv := NewServer(strings.NewReader(input), &out)
	srv.Run()

	responses := parseResponses(t, out.Bytes())
	if len(responses) < 1 {
		t.Fatal("expected at least 1 response")
	}
	if responses[0].Error == nil {
		t.Error("expected error for session/new without initialize")
	}
}

func TestUnknownMethod(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"foo/bar","params":{}}` + "\n"

	var out bytes.Buffer
	srv := NewServer(strings.NewReader(input), &out)
	srv.Run()

	responses := parseResponses(t, out.Bytes())
	if len(responses) < 1 {
		t.Fatal("expected at least 1 response")
	}
	if responses[0].Error == nil {
		t.Error("expected error for unknown method")
	}
	if responses[0].Error.Code != ErrCodeMethodNotFound {
		t.Errorf("expected error code %d, got %d", ErrCodeMethodNotFound, responses[0].Error.Code)
	}
}

func TestSessionList(t *testing.T) {
	lines := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp","mcpServers":[]}}`,
		`{"jsonrpc":"2.0","id":3,"method":"session/list","params":{}}`,
	}
	input := strings.Join(lines, "\n") + "\n"

	var out bytes.Buffer
	srv := NewServer(strings.NewReader(input), &out)
	srv.Run()

	responses := parseResponses(t, out.Bytes())
	if len(responses) < 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}

	resp := responses[2]
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var result ListSessionsResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(result.Sessions))
	}
}

func TestClassifyTool(t *testing.T) {
	tests := []struct {
		name string
		want ToolKind
	}{
		{"bash", ToolKindExecute},
		{"edit", ToolKindEdit},
		{"multi_edit", ToolKindEdit},
		{"write", ToolKindEdit},
		{"view", ToolKindRead},
		{"glob", ToolKindRead},
		{"grep", ToolKindSearch},
		{"fetch_url", ToolKindFetch},
		{"think", ToolKindThink},
		{"delegate", ToolKindOther},
		{"unknown_tool", ToolKindOther},
	}
	for _, tt := range tests {
		got := classifyTool(tt.name)
		if got != tt.want {
			t.Errorf("classifyTool(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestIsMutatingTool(t *testing.T) {
	mutating := []string{"bash", "edit", "multi_edit", "write"}
	for _, name := range mutating {
		if !isMutatingTool(name) {
			t.Errorf("expected %q to be mutating", name)
		}
	}
	readonly := []string{"view", "glob", "grep", "ls", "fetch_url", "think"}
	for _, name := range readonly {
		if isMutatingTool(name) {
			t.Errorf("expected %q to be read-only", name)
		}
	}
}

func TestExtractText(t *testing.T) {
	blocks := []ContentBlock{
		{Type: "text", Text: "hello"},
		{Type: "image", Data: "..."},
		{Type: "text", Text: "world"},
	}
	got := extractText(blocks)
	if got != "hello\nworld" {
		t.Errorf("extractText = %q, want %q", got, "hello\nworld")
	}
}

func TestExtractTextEmpty(t *testing.T) {
	got := extractText(nil)
	if got != "" {
		t.Errorf("extractText(nil) = %q, want empty", got)
	}
}

func TestSessionUpdateMarshal(t *testing.T) {
	update := SessionUpdate{
		SessionUpdate: "agent_message_chunk",
		Content:       &ContentBlock{Type: "text", Text: "hello"},
	}
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["sessionUpdate"] != "agent_message_chunk" {
		t.Errorf("expected sessionUpdate=agent_message_chunk, got %v", parsed["sessionUpdate"])
	}
	// Should not contain tool-related fields.
	if _, ok := parsed["toolCallId"]; ok {
		t.Error("agent_message_chunk should not have toolCallId")
	}
}

func TestSessionUpdateToolCallMarshal(t *testing.T) {
	update := SessionUpdate{
		SessionUpdate: "tool_call",
		ToolCallID:    "call_1",
		Title:         "bash",
		Kind:          ToolKindExecute,
		Status:        ToolCallPending,
	}
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["toolCallId"] != "call_1" {
		t.Errorf("expected toolCallId=call_1, got %v", parsed["toolCallId"])
	}
	if parsed["kind"] != "execute" {
		t.Errorf("expected kind=execute, got %v", parsed["kind"])
	}
	// Should not contain content field.
	if _, ok := parsed["content"]; ok {
		t.Error("tool_call should not have content")
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()
	if id1 == id2 {
		t.Error("session IDs should be unique")
	}
	if !strings.HasPrefix(id1, "sess_") {
		t.Errorf("expected prefix sess_, got %q", id1)
	}
}

// parseResponses splits newline-delimited JSON responses.
func parseResponses(t *testing.T, data []byte) []Response {
	t.Helper()
	var responses []Response
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			t.Fatalf("failed to parse response line: %v\nline: %s", err, line)
		}
		responses = append(responses, resp)
	}
	return responses
}
