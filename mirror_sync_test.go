package main

import (
    "bytes"
    "os"
    "testing"
)

func TestMirroredInternalFilesStayInSync(t *testing.T) {
    pairs := [][2]string{
        {"internal/agent/agent.go", "internal/internal/agent/agent.go"},
        {"internal/agent/runtime_state.go", "internal/internal/agent/runtime_state.go"},
        {"internal/agent/runtime_report.go", "internal/internal/agent/runtime_report.go"},
        {"internal/agent/mcp.go", "internal/internal/agent/mcp.go"},
        {"internal/agent/memory.go", "internal/internal/agent/memory.go"},
        {"internal/agent/hooks.go", "internal/internal/agent/hooks.go"},
        {"internal/agent/session.go", "internal/internal/agent/session.go"},
        {"internal/agent/tools/view.go", "internal/internal/agent/tools/view.go"},
        {"internal/ui/review_regressions_test.go", "internal/internal/ui/review_regressions_test.go"},
    }

    for _, pair := range pairs {
        want, err := os.ReadFile(pair[0])
        if err != nil {
            t.Fatalf("read %s: %v", pair[0], err)
        }
        got, err := os.ReadFile(pair[1])
        if err != nil {
            t.Fatalf("read %s: %v", pair[1], err)
        }
        if !bytes.Equal(got, want) {
            t.Fatalf("mirror drift: %s != %s", pair[1], pair[0])
        }
    }
}
