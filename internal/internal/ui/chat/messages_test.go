package chat

import "testing"

func TestFindToolCallFor_ByCallID(t *testing.T) {
	call1 := &Message{Kind: KindToolCall, CallID: "c1", ToolName: "view"}
	call2 := &Message{Kind: KindToolCall, CallID: "c2", ToolName: "view"}
	call3 := &Message{Kind: KindToolCall, CallID: "c3", ToolName: "view"}
	res2 := &Message{Kind: KindToolResult, CallID: "c2", ToolName: "view", Content: "result2"}
	res3 := &Message{Kind: KindToolResult, CallID: "c3", ToolName: "view", Content: "result3"}
	res1 := &Message{Kind: KindToolResult, CallID: "c1", ToolName: "view", Content: "result1"}

	all := []*Message{call1, call2, call3, res2, res3, res1}

	if got := findToolCallFor(res1, all); got != call1 {
		t.Errorf("res1: expected call1, got %+v", got)
	}
	if got := findToolCallFor(res2, all); got != call2 {
		t.Errorf("res2: expected call2, got %+v", got)
	}
	if got := findToolCallFor(res3, all); got != call3 {
		t.Errorf("res3: expected call3, got %+v", got)
	}
}

func TestFindToolCallFor_ByCallID_MixedTools(t *testing.T) {
	viewCall := &Message{Kind: KindToolCall, CallID: "c1", ToolName: "view"}
	editCall := &Message{Kind: KindToolCall, CallID: "c2", ToolName: "edit"}
	editRes := &Message{Kind: KindToolResult, CallID: "c2", ToolName: "edit", Content: "ok"}
	viewRes := &Message{Kind: KindToolResult, CallID: "c1", ToolName: "view", Content: "ok"}

	all := []*Message{viewCall, editCall, editRes, viewRes}

	if got := findToolCallFor(viewRes, all); got != viewCall {
		t.Errorf("viewRes: expected viewCall, got %+v", got)
	}
	if got := findToolCallFor(editRes, all); got != editCall {
		t.Errorf("editRes: expected editCall, got %+v", got)
	}
}

func TestFindToolCallFor_NoCallID_Fallback(t *testing.T) {
	// Without call IDs, rank-based pairing matches:
	// rank 0 (first result) → last call walking backward
	// rank 1 (second result) → second-to-last call
	call1 := &Message{Kind: KindToolCall, ToolName: "view"}
	call2 := &Message{Kind: KindToolCall, ToolName: "view"}
	res1 := &Message{Kind: KindToolResult, ToolName: "view", Content: "r1"}
	res2 := &Message{Kind: KindToolResult, ToolName: "view", Content: "r2"}

	all := []*Message{call1, call2, res1, res2}

	if got := findToolCallFor(res1, all); got != call2 {
		t.Errorf("res1 (rank 0): expected call2 (nearest), got %+v", got)
	}
	if got := findToolCallFor(res2, all); got != call1 {
		t.Errorf("res2 (rank 1): expected call1 (second nearest), got %+v", got)
	}
}

func TestFindToolCallFor_NoMatch(t *testing.T) {
	res := &Message{Kind: KindToolResult, CallID: "orphan", ToolName: "view", Content: "data"}
	all := []*Message{res}

	if got := findToolCallFor(res, all); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestLongestCommonDirPrefix(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  string
	}{
		{"empty", nil, ""},
		{"single", []string{"/a/b/c.go"}, "/a/b/"},
		{"same dir", []string{"/a/b/c.go", "/a/b/d.go"}, "/a/b/"},
		{"nested", []string{"/a/b/c/d.go", "/a/b/e/f.go"}, "/a/b/"},
		{"root common", []string{"/x/y.go", "/z/w.go"}, "/"},
		{"relative", []string{"src/a.go", "src/b.go"}, "src/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := longestCommonDirPrefix(tt.paths)
			if got != tt.want {
				t.Errorf("longestCommonDirPrefix(%v) = %q, want %q", tt.paths, got, tt.want)
			}
		})
	}
}

func TestParseGrepLine(t *testing.T) {
	tests := []struct {
		line    string
		path    string
		lineNum string
		code    string
		ok      bool
	}{
		{"src/main.go:42: func main() {", "src/main.go", "42", "func main() {", true},
		{"src/main.go:42:func main() {", "src/main.go", "42", "func main() {", true},
		{"no match here", "", "", "", false},
		{"file.go:abc: not a number", "", "", "", false},
		{"", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			path, num, code, ok := parseGrepLine(tt.line)
			if ok != tt.ok || path != tt.path || num != tt.lineNum || code != tt.code {
				t.Errorf("parseGrepLine(%q) = (%q, %q, %q, %v), want (%q, %q, %q, %v)",
					tt.line, path, num, code, ok, tt.path, tt.lineNum, tt.code, tt.ok)
			}
		})
	}
}
