package ui

import (
	"context"

	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/golem/internal/mission"
	"github.com/fugue-labs/golem/internal/skills"
	"github.com/fugue-labs/gollem/core"
)

// gollemSpawner implements mission.AgentSpawner using the gollem agent framework.
// Each spawned agent gets its own model client and runs in the specified worktree.
type gollemSpawner struct {
	baseCfg *config.Config
	skills  []skills.Skill
}

// newGollemSpawner creates a spawner that clones the base config for each agent,
// overriding WorkingDir to the task's worktree path.
func newGollemSpawner(cfg *config.Config, activeSkills []skills.Skill) *gollemSpawner {
	return &gollemSpawner{
		baseCfg: cfg,
		skills:  activeSkills,
	}
}

func (s *gollemSpawner) SpawnWorker(ctx context.Context, spec *mission.WorkerSpec) (mission.AgentHandle, error) {
	cfg := s.workerConfig(spec.WorktreePath)
	a, _, err := agent.New(cfg, spec.Prompt, s.skills)
	if err != nil {
		return nil, err
	}
	return &gollemHandle{agent: a, ctx: ctx, prompt: spec.Prompt}, nil
}

func (s *gollemSpawner) SpawnReviewer(ctx context.Context, spec *mission.ReviewSpec) (mission.AgentHandle, error) {
	cfg := s.reviewerConfig()
	a, _, err := agent.New(cfg, spec.Prompt, s.skills)
	if err != nil {
		return nil, err
	}
	return &gollemHandle{agent: a, ctx: ctx, prompt: spec.Prompt}, nil
}

// workerConfig clones the base config with the worktree as working directory.
func (s *gollemSpawner) workerConfig(worktreePath string) *config.Config {
	cfg := *s.baseCfg
	cfg.WorkingDir = worktreePath
	return &cfg
}

// reviewerConfig clones the base config for a reviewer agent.
// Reviewers run in the main repo (they only read code and diffs).
func (s *gollemSpawner) reviewerConfig() *config.Config {
	cfg := *s.baseCfg
	return &cfg
}

// gollemHandle wraps a gollem agent for the orchestrator's AgentHandle interface.
type gollemHandle struct {
	agent  *core.Agent[string]
	ctx    context.Context
	prompt string
}

func (h *gollemHandle) Wait() (string, error) {
	result, err := h.agent.Run(h.ctx, h.prompt)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}
