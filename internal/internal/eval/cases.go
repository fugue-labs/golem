package eval

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

//go:embed testdata/benchmark_cases.json
var defaultCasesData []byte

type EvalCase struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Prompt      string   `json:"prompt"`
	Provider    string   `json:"provider,omitempty"`
	Model       string   `json:"model,omitempty"`
	ExpectTools []string `json:"expect_tools,omitempty"`
	Notes       string   `json:"notes,omitempty"`
}

func LoadCases(path string) ([]EvalCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading eval cases: %w", err)
	}
	return parseCases(data)
}

func parseCases(data []byte) ([]EvalCase, error) {
	var cases []EvalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("parsing eval cases: %w", err)
	}
	for i := range cases {
		cases[i].ID = strings.TrimSpace(cases[i].ID)
		cases[i].Category = strings.TrimSpace(cases[i].Category)
		cases[i].Prompt = strings.TrimSpace(cases[i].Prompt)
		if cases[i].ID == "" || cases[i].Category == "" || cases[i].Prompt == "" {
			return nil, fmt.Errorf("eval case %d is missing required fields", i)
		}
	}
	return cases, nil
}

func LoadDefaultCases() ([]EvalCase, error) {
	return parseCases(defaultCasesData)
}
