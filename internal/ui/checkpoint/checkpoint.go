package checkpoint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/fugue-labs/golem/internal/ui/chat"
	uiinvariants "github.com/fugue-labs/golem/internal/ui/invariants"
	"github.com/fugue-labs/golem/internal/ui/plan"
	uiverification "github.com/fugue-labs/golem/internal/ui/verification"
	"github.com/fugue-labs/gollem/core"
)

// Checkpoint captures the full session state at one point in time (after an agent turn).
type Checkpoint struct {
	// Turn number (1-indexed, increments with each agent completion).
	Turn int

	// Timestamp of when the checkpoint was created.
	CreatedAt time.Time

	// The user prompt that initiated this turn.
	Prompt string

	// Conversation history (gollem internal messages).
	History []core.ModelMessage

	// UI-visible messages at this point.
	Messages []*chat.Message

	// Tool state snapshot.
	ToolState map[string]any

	// Derived state snapshots.
	PlanState         plan.State
	InvariantState    uiinvariants.State
	VerificationState uiverification.State

	// Usage tracking.
	SessionUsage core.RunUsage
	LastCost     float64

	// Git working tree snapshot (stash object hash, empty if no changes).
	GitStash string
}

// Store manages an ordered list of checkpoints.
type Store struct {
	checkpoints []Checkpoint
	workDir     string
}

// NewStore creates a checkpoint store for the given working directory.
func NewStore(workDir string) *Store {
	return &Store{workDir: workDir}
}

// Save creates a new checkpoint capturing the current state.
// Messages are deep-copied to prevent mutation.
func (s *Store) Save(cp Checkpoint) {
	// Deep-copy messages so later mutations don't affect the checkpoint.
	cp.Messages = copyMessages(cp.Messages)
	cp.ToolState = copyToolState(cp.ToolState)

	// Capture git working tree snapshot.
	cp.GitStash = createGitSnapshot(s.workDir)
	cp.CreatedAt = time.Now()

	s.checkpoints = append(s.checkpoints, cp)
}

// Len returns the number of stored checkpoints.
func (s *Store) Len() int {
	return len(s.checkpoints)
}

// Get returns the checkpoint at the given turn number (1-indexed).
// Returns nil if not found.
func (s *Store) Get(turn int) *Checkpoint {
	for i := range s.checkpoints {
		if s.checkpoints[i].Turn == turn {
			return &s.checkpoints[i]
		}
	}
	return nil
}

// Latest returns the most recent checkpoint, or nil if none exist.
func (s *Store) Latest() *Checkpoint {
	if len(s.checkpoints) == 0 {
		return nil
	}
	return &s.checkpoints[len(s.checkpoints)-1]
}

// RewindTo restores the checkpoint at the given turn number.
// It truncates the checkpoint history to that point and restores the git working tree.
// Returns the checkpoint or an error.
func (s *Store) RewindTo(turn int) (*Checkpoint, error) {
	idx := -1
	for i := range s.checkpoints {
		if s.checkpoints[i].Turn == turn {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("no checkpoint at turn %d (have turns 1–%d)", turn, s.Len())
	}

	cp := s.checkpoints[idx]

	// Restore git working tree: first clean, then apply stash if any.
	if err := restoreGitSnapshot(s.workDir, cp.GitStash); err != nil {
		return nil, fmt.Errorf("restoring files: %w", err)
	}

	// Truncate checkpoint history to this point (keep only turns up to and including idx).
	s.checkpoints = s.checkpoints[:idx+1]

	return &cp, nil
}

// Summary returns a compact description for display.
func (cp *Checkpoint) Summary() string {
	elapsed := cp.CreatedAt.Format("15:04:05")
	prompt := cp.Prompt
	if len(prompt) > 60 {
		prompt = prompt[:57] + "..."
	}
	return fmt.Sprintf("turn %d [%s] %q", cp.Turn, elapsed, prompt)
}

// List returns a summary of all checkpoints for display.
func (s *Store) List() []string {
	summaries := make([]string, len(s.checkpoints))
	for i := range s.checkpoints {
		summaries[i] = s.checkpoints[i].Summary()
	}
	return summaries
}

// Clear removes all checkpoints.
func (s *Store) Clear() {
	s.checkpoints = nil
}

// createGitSnapshot creates a stash-like commit of the current working tree state
// without modifying the working tree. Returns the commit hash or empty string.
func createGitSnapshot(workDir string) string {
	if workDir == "" {
		workDir = "."
	}
	cmd := exec.Command("git", "stash", "create")
	cmd.Dir = workDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	hash := strings.TrimSpace(out.String())
	return hash // empty if working tree is clean
}

// restoreGitSnapshot restores the working tree to match a checkpoint's state.
// If stashHash is empty, the working tree was clean at that checkpoint — reset to HEAD.
// If stashHash is non-empty, reset to HEAD then apply the stash.
func restoreGitSnapshot(workDir string, stashHash string) error {
	if workDir == "" {
		workDir = "."
	}

	// First, discard all current working tree changes to get a clean state.
	resetCmd := exec.Command("git", "checkout", "--", ".")
	resetCmd.Dir = workDir
	var resetOut bytes.Buffer
	resetCmd.Stdout = &resetOut
	resetCmd.Stderr = &resetOut
	if err := resetCmd.Run(); err != nil {
		return fmt.Errorf("git checkout -- . failed: %w: %s", err, resetOut.String())
	}

	// Also clean untracked files that were created after the checkpoint.
	// Use git clean -fd to remove untracked files and directories.
	cleanCmd := exec.Command("git", "clean", "-fd")
	cleanCmd.Dir = workDir
	var cleanOut bytes.Buffer
	cleanCmd.Stdout = &cleanOut
	cleanCmd.Stderr = &cleanOut
	_ = cleanCmd.Run() // Best effort — clean might not be needed

	if stashHash == "" {
		// Working tree was clean at checkpoint — we're done.
		return nil
	}

	// Apply the stash snapshot to restore the exact working tree state.
	applyCmd := exec.Command("git", "stash", "apply", "--quiet", stashHash)
	applyCmd.Dir = workDir
	var applyOut bytes.Buffer
	applyCmd.Stdout = &applyOut
	applyCmd.Stderr = &applyOut
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("git stash apply failed: %w: %s", err, applyOut.String())
	}

	return nil
}

// copyMessages creates a shallow copy of the message slice with each message copied.
func copyMessages(msgs []*chat.Message) []*chat.Message {
	if msgs == nil {
		return nil
	}
	cp := make([]*chat.Message, len(msgs))
	for i, msg := range msgs {
		dup := *msg
		cp[i] = &dup
	}
	return cp
}

// copyToolState creates a deep copy of tool state via JSON round-trip.
func copyToolState(ts map[string]any) map[string]any {
	if ts == nil {
		return nil
	}
	data, err := json.Marshal(ts)
	if err != nil {
		return ts // fallback to shallow ref
	}
	var cp map[string]any
	if err := json.Unmarshal(data, &cp); err != nil {
		return ts
	}
	return cp
}
