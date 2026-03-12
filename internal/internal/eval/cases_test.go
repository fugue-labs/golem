package eval

import "testing"

func TestLoadDefaultCases(t *testing.T) {
	cases, err := LoadDefaultCases()
	if err != nil {
		t.Fatalf("LoadDefaultCases() error = %v", err)
	}
	if len(cases) < 4 {
		t.Fatalf("cases=%d, want at least 4", len(cases))
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.ID] {
			t.Fatalf("duplicate eval case id %q", c.ID)
		}
		seen[c.ID] = true
		if c.Category == "" || c.Prompt == "" {
			t.Fatalf("invalid eval case: %+v", c)
		}
	}
}
