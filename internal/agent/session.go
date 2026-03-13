package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// SessionData holds the full serializable state of a golem session.
type SessionData struct {
	Messages          json.RawMessage        `json:"messages"`
	Transcript        json.RawMessage        `json:"transcript,omitempty"`
	ToolState         map[string]any         `json:"tool_state,omitempty"`
	Usage             core.RunUsage          `json:"usage"`
	Model             string                 `json:"model"`
	Provider          string                 `json:"provider"`
	WorkDir           string                 `json:"work_dir"`
	Timestamp         time.Time              `json:"timestamp"`
	Prompt            string                 `json:"prompt,omitempty"`
	PlanState         json.RawMessage        `json:"plan_state,omitempty"`
	InvariantState    json.RawMessage        `json:"invariant_state,omitempty"`
	VerificationState json.RawMessage        `json:"verification_state,omitempty"`
	SpecState         json.RawMessage        `json:"spec_state,omitempty"`
}

// SessionDir returns the session directory for the given working directory.
// Sessions are stored in ~/.golem/sessions/<project-hash>/.
func SessionDir(workDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	hash := projectHash(workDir)
	dir := filepath.Join(home, ".golem", "sessions", hash)
	return dir, nil
}

// SaveSession writes the current session state to disk.
func SaveSession(workDir string, messages []core.ModelMessage, transcript any, toolState map[string]any, usage core.RunUsage, model, provider, prompt string, planState, invariantState, verificationState, specState any) error {
	dir, err := SessionDir(workDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	msgData, err := core.MarshalMessages(messages)
	if err != nil {
		return fmt.Errorf("marshaling messages: %w", err)
	}

	data := SessionData{
		Messages:  msgData,
		ToolState: toolState,
		Usage:     usage,
		Model:     model,
		Provider:  provider,
		WorkDir:   workDir,
		Timestamp: time.Now(),
		Prompt:    prompt,
	}

	if transcript != nil {
		if transcriptData, err := json.Marshal(transcript); err == nil {
			data.Transcript = transcriptData
		} else {
			return fmt.Errorf("marshaling transcript: %w", err)
		}
	}
	if planState != nil {
		if planData, err := json.Marshal(planState); err == nil {
			data.PlanState = planData
		} else {
			return fmt.Errorf("marshaling plan state: %w", err)
		}
	}
	if invariantState != nil {
		if invariantData, err := json.Marshal(invariantState); err == nil {
			data.InvariantState = invariantData
		} else {
			return fmt.Errorf("marshaling invariant state: %w", err)
		}
	}
	if verificationState != nil {
		if verificationData, err := json.Marshal(verificationState); err == nil {
			data.VerificationState = verificationData
		} else {
			return fmt.Errorf("marshaling verification state: %w", err)
		}
	}
	if specState != nil {
		if specData, err := json.Marshal(specState); err == nil {
			data.SpecState = specData
		} else {
			return fmt.Errorf("marshaling spec state: %w", err)
		}
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s.json", time.Now().Format("2006-01-02T15-04-05"))
	return os.WriteFile(filepath.Join(dir, filename), raw, 0644)
}

// LoadLatestSession loads the most recent session for the given working directory.
// Returns nil, nil if no session exists.
func LoadLatestSession(workDir string) (*SessionData, error) {
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

	// Find JSON files and sort by name (timestamp-based, so lexicographic = chronological).
	var jsonFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			jsonFiles = append(jsonFiles, e.Name())
		}
	}
	if len(jsonFiles) == 0 {
		return nil, nil
	}
	sort.Strings(jsonFiles)
	latest := jsonFiles[len(jsonFiles)-1]

	raw, err := os.ReadFile(filepath.Join(dir, latest))
	if err != nil {
		return nil, err
	}

	var data SessionData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// RestoreMessages deserializes the messages from a session.
func (s *SessionData) RestoreMessages() ([]core.ModelMessage, error) {
	return core.UnmarshalMessages(s.Messages)
}

// RestoreJSON decodes an optional JSON payload into out, leaving out unchanged
// when the payload is absent so older session files continue to load.
func RestoreJSON[T any](raw json.RawMessage, out *T) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// projectHash returns a short hash of the working directory for session grouping.
func projectHash(workDir string) string {
	h := sha256.Sum256([]byte(workDir))
	return hex.EncodeToString(h[:8])
}
