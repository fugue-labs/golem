package mission

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// AgentHandle represents a running agent that can be waited on for completion.
type AgentHandle interface {
	// Wait blocks until the agent finishes and returns its output summary.
	Wait() (summary string, err error)
}

// AgentSpawner creates agent processes for worker and reviewer tasks.
// The production implementation wraps gollem's agent creation and execution.
// Tests provide a mock that completes synchronously.
type AgentSpawner interface {
	SpawnWorker(ctx context.Context, spec *WorkerSpec) (AgentHandle, error)
	SpawnReviewer(ctx context.Context, spec *ReviewSpec) (AgentHandle, error)
}

// OrchestratorEvent is emitted for each notable orchestration lifecycle change.
// The TUI registers a callback to receive these and update the display.
type OrchestratorEvent struct {
	Type      string // "worker.started", "worker.completed", "review.passed", "mission.completed", etc.
	MissionID string
	TaskID    string
	RunID     string
	Message   string
	Error     error
}

// OrchestratorConfig holds parameters for the orchestration loop.
type OrchestratorConfig struct {
	MissionID         string
	RepoRoot          string
	TickInterval      time.Duration // How often to check for work. Default: 5s.
	MaxAttempts       int           // Max attempts per task before permanent failure. Default: 3.
	HeartbeatInterval time.Duration // How often to heartbeat running workers. Default: 5m.

	// Retry/resilience settings for Dolt store operations.
	StoreRetryAttempts         int           // Retries per store call within a tick. Default: 3.
	StoreRetryBaseDelay        time.Duration // Initial backoff between retries. Default: 100ms.
	MaxConsecutiveStoreFailures int           // Consecutive failed ticks before mission error. Default: 5.
}

func (c *OrchestratorConfig) applyDefaults() {
	if c.TickInterval <= 0 {
		c.TickInterval = 5 * time.Second
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = 5 * time.Minute
	}
	if c.StoreRetryAttempts <= 0 {
		c.StoreRetryAttempts = 3
	}
	if c.StoreRetryBaseDelay <= 0 {
		c.StoreRetryBaseDelay = 100 * time.Millisecond
	}
	if c.MaxConsecutiveStoreFailures <= 0 {
		c.MaxConsecutiveStoreFailures = 5
	}
}

// ---------------------------------------------------------------------------
// Internal interfaces — allow testing without git operations
// ---------------------------------------------------------------------------

type workerDispatcher interface {
	DispatchReadyTasks(ctx context.Context, missionID string) ([]*WorkerSpec, error)
	CompleteWorker(ctx context.Context, spec *WorkerSpec, summary string) error
	FailWorker(ctx context.Context, spec *WorkerSpec, errText string, maxAttempts int) error
	HeartbeatWorker(ctx context.Context, spec *WorkerSpec) error
	ReleaseWorkerWorktree(ctx context.Context, missionID, taskID string)
}

type reviewDispatcher interface {
	DispatchPendingReviews(ctx context.Context, missionID, repoRoot string) ([]*ReviewSpec, error)
	CompleteReview(ctx context.Context, spec *ReviewSpec, result *ReviewResult) error
	FailReview(ctx context.Context, spec *ReviewSpec, errText string) error
}

type integrator interface {
	IntegrateReady(ctx context.Context, missionID string) ([]*IntegrationResult, error)
	CheckMissionComplete(ctx context.Context, missionID string) (bool, error)
	CompleteMission(ctx context.Context, missionID string) error
}

// Compile-time checks that concrete types satisfy the internal interfaces.
var (
	_ workerDispatcher = (*WorkerLauncher)(nil)
	_ reviewDispatcher = (*ReviewLauncher)(nil)
	_ integrator       = (*IntegrationEngine)(nil)
)

// ---------------------------------------------------------------------------
// Orchestrator
// ---------------------------------------------------------------------------

type activeAgent struct {
	runID  string
	taskID string
	mode   RunMode
	cancel context.CancelFunc
}

// Orchestrator drives a mission to completion by continuously dispatching
// workers, reviewing their output, integrating accepted changes, and resolving
// dependencies — all in a background tick loop.
//
// Each tick:
//  1. Dispatches workers for ready tasks (via WorkerLauncher + Scheduler).
//  2. Dispatches reviewers for tasks awaiting review.
//  3. Integrates accepted tasks into the base branch.
//  4. Checks if the mission is complete.
//
// Worker and reviewer agents run as concurrent goroutines, each monitored by
// a heartbeat loop. The orchestrator stops when the mission reaches a terminal
// state, the context is cancelled, or Stop() is called.
type Orchestrator struct {
	cfg     OrchestratorConfig
	store   Store
	spawner AgentSpawner
	workers workerDispatcher
	reviews reviewDispatcher
	integr  integrator
	onEvent func(OrchestratorEvent)
	logger  *slog.Logger

	mu                       sync.Mutex
	active                   map[string]*activeAgent // runID → active agent
	ctx                      context.Context
	cancel                   context.CancelFunc
	done                     chan struct{}
	doneOnce                 sync.Once
	started                  bool
	stopped                  bool
	consecutiveStoreFailures int // ticks where all store ops failed
}

// NewOrchestrator creates an orchestrator wired to the concrete mission
// components. It constructs the Scheduler, WorkerLauncher, ReviewLauncher,
// and IntegrationEngine internally from the given store and worktree manager.
func NewOrchestrator(
	cfg OrchestratorConfig,
	store Store,
	spawner AgentSpawner,
	worktrees *WorktreeManager,
	onEvent func(OrchestratorEvent),
) *Orchestrator {
	cfg.applyDefaults()
	scheduler := NewScheduler(store)
	return &Orchestrator{
		cfg:     cfg,
		store:   store,
		spawner: spawner,
		workers: NewWorkerLauncher(scheduler, worktrees, store),
		reviews: NewReviewLauncher(store),
		integr:  NewIntegrationEngine(store, cfg.RepoRoot),
		onEvent: onEvent,
		logger:  slog.Default(),
		active:  make(map[string]*activeAgent),
		done:    make(chan struct{}),
	}
}

// Start begins the orchestration loop in a background goroutine.
// The loop runs until the context is cancelled, Stop() is called,
// or the mission reaches a terminal state.
func (o *Orchestrator) Start(ctx context.Context) {
	o.mu.Lock()
	if o.stopped {
		o.mu.Unlock()
		o.closeDone()
		return
	}
	o.ctx, o.cancel = context.WithCancel(ctx)
	o.started = true
	o.mu.Unlock()
	go o.loop()
}

// Stop cancels all in-flight agents and waits for the loop to exit.
// Safe to call even if Start() was never called or is running concurrently.
func (o *Orchestrator) Stop() {
	o.mu.Lock()
	o.stopped = true
	cancel := o.cancel
	started := o.started
	o.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if !started {
		o.closeDone()
		return // loop never ran, nothing to wait for
	}

	select {
	case <-o.done:
	case <-time.After(30 * time.Second):
	}
}

// Wait blocks until the orchestrator exits (mission completed or cancelled).
func (o *Orchestrator) Wait() {
	<-o.done
}

// ActiveRunCount returns the number of currently running agent goroutines.
func (o *Orchestrator) ActiveRunCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.active)
}

// ---------------------------------------------------------------------------
// Main loop
// ---------------------------------------------------------------------------

func (o *Orchestrator) closeDone() {
	o.doneOnce.Do(func() { close(o.done) })
}

func (o *Orchestrator) loop() {
	defer o.closeDone()

	// Run first tick immediately.
	o.tick()

	ticker := time.NewTicker(o.cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			o.drainAgents()
			return
		case <-ticker.C:
			o.tick()
		}
	}
}

func (o *Orchestrator) storeRetryConfig() retryConfig {
	return retryConfig{
		MaxAttempts: o.cfg.StoreRetryAttempts,
		BaseDelay:   o.cfg.StoreRetryBaseDelay,
	}
}

func (o *Orchestrator) tick() {
	ctx := o.ctx
	if ctx.Err() != nil {
		return
	}

	retryCfg := o.storeRetryConfig()

	// Check mission is still running (with retry for transient Dolt errors).
	m, err := retryStoreGet(ctx, retryCfg, func(c context.Context) (*Mission, error) {
		return o.store.GetMission(c, o.cfg.MissionID)
	})
	if err != nil {
		if ctx.Err() != nil {
			return // shutting down
		}
		o.recordStoreFailure(err)
		return
	}
	if m.Status.IsTerminal() {
		o.cancel()
		return
	}
	if m.Status != MissionRunning {
		return // paused, blocked, etc — skip this tick
	}

	// Reset consecutive failure counter — we successfully read mission state.
	o.resetStoreFailures()

	// Phase A: Dispatch workers for ready tasks.
	o.dispatchWorkers(ctx)

	// Phase B: Dispatch reviewers for tasks awaiting review.
	o.dispatchReviewers(ctx)

	// Phase C: Integrate accepted tasks.
	o.integrateAccepted(ctx)

	// Phase D: Check if mission is complete.
	o.checkCompletion(ctx)
}

// recordStoreFailure tracks consecutive store failures. After exceeding the
// threshold, it sets the mission to failed state so operators are alerted.
func (o *Orchestrator) recordStoreFailure(err error) {
	o.mu.Lock()
	o.consecutiveStoreFailures++
	failures := o.consecutiveStoreFailures
	threshold := o.cfg.MaxConsecutiveStoreFailures
	o.mu.Unlock()

	if IsTransientError(err) {
		o.logger.Warn("orchestrator: transient store error",
			"error", err,
			"consecutive_failures", failures,
			"threshold", threshold,
		)
		o.emit(OrchestratorEvent{
			Type:    "store.transient_error",
			Message: fmt.Sprintf("transient store error (attempt %d/%d): %v", failures, threshold, err),
			Error:   err,
		})
	} else {
		o.logger.Error("orchestrator: permanent store error", "error", err)
		o.emit(OrchestratorEvent{
			Type:    "store.error",
			Message: fmt.Sprintf("permanent store error: %v", err),
			Error:   err,
		})
	}

	if failures >= threshold {
		o.logger.Error("orchestrator: store failures exceeded threshold, failing mission",
			"consecutive_failures", failures,
			"threshold", threshold,
		)
		o.emit(OrchestratorEvent{
			Type:    "mission.store_failure",
			Message: fmt.Sprintf("mission failed: %d consecutive store failures", failures),
			Error:   err,
		})
		o.cancel()
	}
}

func (o *Orchestrator) resetStoreFailures() {
	o.mu.Lock()
	o.consecutiveStoreFailures = 0
	o.mu.Unlock()
}

// ConsecutiveStoreFailures returns the current consecutive failure count
// (for testing and observability).
func (o *Orchestrator) ConsecutiveStoreFailures() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.consecutiveStoreFailures
}

// ---------------------------------------------------------------------------
// Phase A: Worker dispatch
// ---------------------------------------------------------------------------

func (o *Orchestrator) dispatchWorkers(ctx context.Context) {
	specs, err := o.workers.DispatchReadyTasks(ctx, o.cfg.MissionID)
	if err != nil {
		o.logger.Warn("orchestrator: dispatch workers", "error", err)
		return
	}
	for _, spec := range specs {
		o.spawnWorker(ctx, spec)
	}
}

func (o *Orchestrator) spawnWorker(ctx context.Context, spec *WorkerSpec) {
	agentCtx, agentCancel := context.WithCancel(ctx)

	handle, err := o.spawner.SpawnWorker(agentCtx, spec)
	if err != nil {
		agentCancel()
		o.workers.FailWorker(ctx, spec, fmt.Sprintf("spawn failed: %v", err), o.cfg.MaxAttempts) //nolint:errcheck
		o.emit(OrchestratorEvent{
			Type:   "worker.spawn_failed",
			TaskID: spec.Task.ID,
			RunID:  spec.Run.ID,
			Error:  err,
		})
		return
	}

	o.mu.Lock()
	o.active[spec.Run.ID] = &activeAgent{
		runID:  spec.Run.ID,
		taskID: spec.Task.ID,
		mode:   RunModeWorker,
		cancel: agentCancel,
	}
	o.mu.Unlock()

	o.emit(OrchestratorEvent{
		Type:    "worker.started",
		TaskID:  spec.Task.ID,
		RunID:   spec.Run.ID,
		Message: spec.Task.Title,
	})

	go o.runWorker(agentCtx, agentCancel, spec, handle)
}

func (o *Orchestrator) runWorker(ctx context.Context, cancel context.CancelFunc, spec *WorkerSpec, handle AgentHandle) {
	defer cancel()
	defer o.removeActive(spec.Run.ID)
	defer func() {
		if r := recover(); r != nil {
			o.logger.Error("runWorker panic recovered",
				"run", spec.Run.ID,
				"task", spec.Task.ID,
				"panic", r,
			)
			o.workers.FailWorker(o.ctx, spec, fmt.Sprintf("panic: %v", r), o.cfg.MaxAttempts) //nolint:errcheck
			o.emit(OrchestratorEvent{
				Type:   "worker.panic",
				TaskID: spec.Task.ID,
				RunID:  spec.Run.ID,
				Error:  fmt.Errorf("panic: %v", r),
			})
		}
	}()

	// Start heartbeat goroutine.
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		o.heartbeatLoop(heartbeatCtx, spec)
	}()

	// Wait for agent completion.
	summary, err := handle.Wait()

	// Stop the heartbeat goroutine and wait for it to finish before
	// accessing spec.Run, preventing a data race between heartbeatLoop
	// modifying Run.HeartbeatAt/LeaseExpires and the code below
	// modifying Run.Status/EndedAt/Summary.
	heartbeatCancel()
	<-heartbeatDone

	if err != nil && ctx.Err() != nil {
		return // worker context cancelled (timeout or independent cancellation)
	}
	if o.ctx.Err() != nil {
		return // orchestrator shutting down
	}

	if err != nil {
		o.workers.FailWorker(o.ctx, spec, err.Error(), o.cfg.MaxAttempts) //nolint:errcheck
		o.emit(OrchestratorEvent{
			Type:   "worker.failed",
			TaskID: spec.Task.ID,
			RunID:  spec.Run.ID,
			Error:  err,
		})
		return
	}

	if err := o.workers.CompleteWorker(o.ctx, spec, summary); err != nil {
		o.emit(OrchestratorEvent{
			Type:   "worker.complete_error",
			TaskID: spec.Task.ID,
			RunID:  spec.Run.ID,
			Error:  err,
		})
		return
	}

	o.emit(OrchestratorEvent{
		Type:    "worker.completed",
		TaskID:  spec.Task.ID,
		RunID:   spec.Run.ID,
		Message: summary,
	})
}

func (o *Orchestrator) heartbeatLoop(ctx context.Context, spec *WorkerSpec) {
	ticker := time.NewTicker(o.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := o.workers.HeartbeatWorker(ctx, spec); err != nil {
				o.logger.Warn("heartbeat failed", "run", spec.Run.ID, "error", err)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Phase B: Review dispatch
// ---------------------------------------------------------------------------

func (o *Orchestrator) dispatchReviewers(ctx context.Context) {
	specs, err := o.reviews.DispatchPendingReviews(ctx, o.cfg.MissionID, o.cfg.RepoRoot)
	if err != nil {
		o.logger.Warn("orchestrator: dispatch reviews", "error", err)
		return
	}
	for _, spec := range specs {
		o.spawnReviewer(ctx, spec)
	}
}

func (o *Orchestrator) spawnReviewer(ctx context.Context, spec *ReviewSpec) {
	agentCtx, agentCancel := context.WithCancel(ctx)

	handle, err := o.spawner.SpawnReviewer(agentCtx, spec)
	if err != nil {
		agentCancel()
		o.reviews.FailReview(ctx, spec, fmt.Sprintf("spawn failed: %v", err)) //nolint:errcheck
		o.workers.ReleaseWorkerWorktree(ctx, spec.Run.MissionID, spec.Task.ID)
		o.emit(OrchestratorEvent{
			Type:   "review.spawn_failed",
			TaskID: spec.Task.ID,
			RunID:  spec.Run.ID,
			Error:  err,
		})
		return
	}

	o.mu.Lock()
	o.active[spec.Run.ID] = &activeAgent{
		runID:  spec.Run.ID,
		taskID: spec.Task.ID,
		mode:   RunModeReview,
		cancel: agentCancel,
	}
	o.mu.Unlock()

	o.emit(OrchestratorEvent{
		Type:    "review.started",
		TaskID:  spec.Task.ID,
		RunID:   spec.Run.ID,
		Message: spec.Task.Title,
	})

	go o.runReviewer(agentCtx, agentCancel, spec, handle)
}

func (o *Orchestrator) runReviewer(ctx context.Context, cancel context.CancelFunc, spec *ReviewSpec, handle AgentHandle) {
	defer cancel()
	defer o.removeActive(spec.Run.ID)
	defer func() {
		if r := recover(); r != nil {
			o.logger.Error("runReviewer panic recovered",
				"run", spec.Run.ID,
				"task", spec.Task.ID,
				"panic", r,
			)
			o.reviews.FailReview(o.ctx, spec, fmt.Sprintf("panic: %v", r)) //nolint:errcheck
			o.emit(OrchestratorEvent{
				Type:   "review.panic",
				TaskID: spec.Task.ID,
				RunID:  spec.Run.ID,
				Error:  fmt.Errorf("panic: %v", r),
			})
		}
	}()

	summary, err := handle.Wait()
	if err != nil && ctx.Err() != nil {
		return // reviewer context cancelled (timeout or independent cancellation)
	}
	if o.ctx.Err() != nil {
		return
	}

	if err != nil {
		o.reviews.FailReview(o.ctx, spec, err.Error()) //nolint:errcheck
		o.emit(OrchestratorEvent{
			Type:   "review.failed",
			TaskID: spec.Task.ID,
			RunID:  spec.Run.ID,
			Error:  err,
		})
		return
	}

	result, parseErr := ParseReviewResult(summary)
	if parseErr != nil {
		o.reviews.FailReview(o.ctx, spec, fmt.Sprintf("parse review result: %v", parseErr)) //nolint:errcheck
		o.emit(OrchestratorEvent{
			Type:   "review.parse_failed",
			TaskID: spec.Task.ID,
			RunID:  spec.Run.ID,
			Error:  parseErr,
		})
		return
	}

	if err := o.reviews.CompleteReview(o.ctx, spec, result); err != nil {
		o.emit(OrchestratorEvent{
			Type:   "review.complete_error",
			TaskID: spec.Task.ID,
			RunID:  spec.Run.ID,
			Error:  err,
		})
		return
	}

	o.emit(OrchestratorEvent{
		Type:    "review." + string(result.Verdict),
		TaskID:  spec.Task.ID,
		RunID:   spec.Run.ID,
		Message: result.Summary,
	})

	// Release worktree based on verdict:
	// - pass: integrator uses the branch, release happens in integrateAccepted
	// - reject: worker starts from scratch, release worktree
	// - request_changes: worker keeps existing worktree to iterate on feedback
	if result.Verdict == ReviewReject {
		o.workers.ReleaseWorkerWorktree(o.ctx, spec.Run.MissionID, spec.Task.ID)
	}
}

// ---------------------------------------------------------------------------
// Phase C: Integration
// ---------------------------------------------------------------------------

func (o *Orchestrator) integrateAccepted(ctx context.Context) {
	results, err := o.integr.IntegrateReady(ctx, o.cfg.MissionID)
	if err != nil {
		o.logger.Warn("orchestrator: integrate", "error", err)
		return
	}
	for _, r := range results {
		// Always release the worker worktree after integration attempt,
		// regardless of success or failure. Failing to release on error
		// causes worktree leaks (orphaned worktrees never cleaned up).
		o.workers.ReleaseWorkerWorktree(ctx, o.cfg.MissionID, r.TaskID)

		if r.Success {
			o.emit(OrchestratorEvent{
				Type:    "integration.completed",
				TaskID:  r.TaskID,
				Message: fmt.Sprintf("merged: %s", r.MergedCommit),
			})
		} else {
			o.emit(OrchestratorEvent{
				Type:    "integration.failed",
				TaskID:  r.TaskID,
				Message: r.ErrorText,
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Phase D: Completion check
// ---------------------------------------------------------------------------

func (o *Orchestrator) checkCompletion(ctx context.Context) {
	complete, err := o.integr.CheckMissionComplete(ctx, o.cfg.MissionID)
	if err != nil {
		o.logger.Warn("orchestrator: check completion", "error", err)
		return
	}
	if !complete {
		return
	}

	if err := o.integr.CompleteMission(ctx, o.cfg.MissionID); err != nil {
		o.logger.Error("orchestrator: complete mission", "error", err)
		return
	}

	o.emit(OrchestratorEvent{
		Type:      "mission.completed",
		MissionID: o.cfg.MissionID,
		Message:   "All tasks integrated successfully",
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (o *Orchestrator) emit(e OrchestratorEvent) {
	if e.MissionID == "" {
		e.MissionID = o.cfg.MissionID
	}
	if o.onEvent != nil {
		o.onEvent(e)
	}
}

func (o *Orchestrator) removeActive(runID string) {
	o.mu.Lock()
	delete(o.active, runID)
	o.mu.Unlock()
}

func (o *Orchestrator) drainAgents() {
	o.mu.Lock()
	for _, a := range o.active {
		a.cancel()
	}
	o.mu.Unlock()

	// Wait briefly for goroutines to notice cancellation.
	deadline := time.After(10 * time.Second)
	for {
		o.mu.Lock()
		n := len(o.active)
		o.mu.Unlock()
		if n == 0 {
			return
		}
		select {
		case <-deadline:
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// ---------------------------------------------------------------------------
// ParseReviewResult extracts a ReviewResult from agent output text.
// ---------------------------------------------------------------------------

// ParseReviewResult parses the reviewer agent's output text to extract
// the structured review verdict. It reuses the same JSON extraction
// logic as ParsePlanResult.
func ParseReviewResult(text string) (*ReviewResult, error) {
	jsonStr, err := extractJSONObject(text)
	if err != nil {
		return nil, fmt.Errorf("extract review JSON: %w", err)
	}

	var result ReviewResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("unmarshal review result: %w", err)
	}
	if result.Verdict == "" {
		return nil, fmt.Errorf("review result missing verdict")
	}

	return &result, nil
}
