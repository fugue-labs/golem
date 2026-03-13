package ui

import (
	"strings"
)

// PaceMode controls how the agent paces its work relative to the human.
type PaceMode string

const (
	PaceOff        PaceMode = "off"        // Default — agent runs freely
	PaceCheckpoint PaceMode = "checkpoint" // Pause every N tool calls
	PacePingPong   PaceMode = "pingpong"   // Approve each mutating tool individually
	PaceReview     PaceMode = "review"     // Agent works, then shows diff for review
)

// paceCheckpointRequest is sent from the agent goroutine (OnToolEnd hook)
// to the TUI when a checkpoint interval is reached. The goroutine blocks
// on the response channel until the user continues.
type paceCheckpointRequest struct {
	runID    int
	count    int             // how many tools were run since last checkpoint
	response chan<- struct{} // close or send to resume the agent
}

// paceState tracks pace-mode runtime state.
type paceState struct {
	mode               PaceMode
	checkpointInterval int  // tool calls between checkpoints (default 5)
	clarifyFirst       bool // ask clarifying questions before executing
}

func defaultPaceState() paceState {
	return paceState{
		mode:               PaceOff,
		checkpointInterval: 5,
	}
}

func parsePaceMode(s string) (PaceMode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off":
		return PaceOff, true
	case "checkpoint", "cp":
		return PaceCheckpoint, true
	case "pingpong", "pp":
		return PacePingPong, true
	case "review", "rev":
		return PaceReview, true
	default:
		return PaceOff, false
	}
}
