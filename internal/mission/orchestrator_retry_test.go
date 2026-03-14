package mission

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// failingStore wraps a real store to inject transient errors
// ---------------------------------------------------------------------------

type failingStore struct {
	Store
	mu               sync.Mutex
	getMissionFails  int32 // atomic: number of GetMission calls that should fail
	getMissionCalled atomic.Int32
	failErr          error
}

func newFailingStore(inner Store, failCount int, err error) *failingStore {
	return &failingStore{
		Store:           inner,
		getMissionFails: int32(failCount),
		failErr:         err,
	}
}

func (s *failingStore) GetMission(ctx context.Context, id string) (*Mission, error) {
	n := s.getMissionCalled.Add(1)
	s.mu.Lock()
	shouldFail := n <= int32(s.getMissionFails)
	s.mu.Unlock()
	if shouldFail {
		return nil, s.failErr
	}
	return s.Store.GetMission(ctx, id)
}

func (s *failingStore) setFailCount(n int) {
	s.mu.Lock()
	s.getMissionFails = int32(n)
	s.mu.Unlock()
}

func (s *failingStore) resetCalls() {
	s.getMissionCalled.Store(0)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestOrchestratorRecoversFromTransientStoreError(t *testing.T) {
	inner := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	inner.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})
	inner.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Task",
		Kind:      TaskKindCode,
		Objective: "Do work",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	})

	// Fail the first 2 GetMission calls with a transient error.
	// With 3 retry attempts per tick, the first tick's retries should
	// succeed on the 3rd attempt within the same tick.
	store := newFailingStore(inner, 2, fmt.Errorf("dial tcp: i/o timeout"))

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	wd := &storeWorkerDispatcher{store: inner}
	orch := &Orchestrator{
		cfg: OrchestratorConfig{
			MissionID:                   "m1",
			RepoRoot:                    "/test/repo",
			TickInterval:                50 * time.Millisecond,
			MaxAttempts:                 3,
			HeartbeatInterval:           time.Minute,
			StoreRetryAttempts:          3,
			StoreRetryBaseDelay:         time.Millisecond,
			MaxConsecutiveStoreFailures: 5,
		},
		store:   store,
		spawner: spawner,
		workers: wd,
		reviews: &storeReviewDispatcher{store: inner},
		integr:  &storeIntegrator{store: inner},
		onEvent: func(e OrchestratorEvent) {
			select {
			case events <- e:
			default:
			}
		},
		logger: slog.Default(),
		active: make(map[string]*activeAgent),
		done:   make(chan struct{}),
	}

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	orch.Start(testCtx)
	defer orch.Stop()

	// The orchestrator should recover and dispatch the worker.
	waitFor(t, 3*time.Second, "worker spawned after transient errors", func() bool {
		return spawner.workerCount() >= 1
	})

	// Consecutive failures should have been reset.
	if orch.ConsecutiveStoreFailures() != 0 {
		t.Fatalf("consecutive failures = %d, want 0 after recovery", orch.ConsecutiveStoreFailures())
	}
}

func TestOrchestratorFailsMissionAfterSustainedStoreErrors(t *testing.T) {
	inner := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	inner.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})

	// Fail ALL GetMission calls — simulates sustained Dolt outage.
	store := newFailingStore(inner, 9999, fmt.Errorf("dial tcp: i/o timeout"))

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch := &Orchestrator{
		cfg: OrchestratorConfig{
			MissionID:                   "m1",
			RepoRoot:                    "/test/repo",
			TickInterval:                20 * time.Millisecond,
			MaxAttempts:                 3,
			HeartbeatInterval:           time.Minute,
			StoreRetryAttempts:          2,
			StoreRetryBaseDelay:         time.Millisecond,
			MaxConsecutiveStoreFailures: 3,
		},
		store:   store,
		spawner: spawner,
		workers: &storeWorkerDispatcher{store: inner},
		reviews: &storeReviewDispatcher{store: inner},
		integr:  &storeIntegrator{store: inner},
		onEvent: func(e OrchestratorEvent) {
			select {
			case events <- e:
			default:
			}
		},
		logger: slog.Default(),
		active: make(map[string]*activeAgent),
		done:   make(chan struct{}),
	}

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	orch.Start(testCtx)

	// Wait for the mission.store_failure event.
	e := waitForEvent(t, events, "mission.store_failure", 5*time.Second)
	if e.Error == nil {
		t.Fatal("expected error in mission.store_failure event")
	}

	// Orchestrator should stop after exceeding the threshold.
	select {
	case <-orch.done:
		// Good — orchestrator exited.
	case <-time.After(5 * time.Second):
		t.Fatal("orchestrator did not exit after sustained store failures")
	}

	// No workers should have been spawned.
	if spawner.workerCount() != 0 {
		t.Fatalf("expected no workers, got %d", spawner.workerCount())
	}
}

func TestOrchestratorTransientErrorEmitsEvents(t *testing.T) {
	inner := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	inner.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})
	inner.CreateTask(ctx, &Task{
		ID:        "t1",
		MissionID: "m1",
		Title:     "Task",
		Kind:      TaskKindCode,
		Objective: "Do work",
		Status:    TaskReady,
		Priority:  1,
		RiskLevel: RiskLow,
		CreatedAt: now,
		UpdatedAt: now,
	})

	// Fail first 6 calls (2 ticks × 3 retries each), then succeed.
	store := newFailingStore(inner, 6, fmt.Errorf("dial tcp: i/o timeout"))

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch := &Orchestrator{
		cfg: OrchestratorConfig{
			MissionID:                   "m1",
			RepoRoot:                    "/test/repo",
			TickInterval:                30 * time.Millisecond,
			MaxAttempts:                 3,
			HeartbeatInterval:           time.Minute,
			StoreRetryAttempts:          3,
			StoreRetryBaseDelay:         time.Millisecond,
			MaxConsecutiveStoreFailures: 10, // high threshold so we don't fail
		},
		store:   store,
		spawner: spawner,
		workers: &storeWorkerDispatcher{store: inner},
		reviews: &storeReviewDispatcher{store: inner},
		integr:  &storeIntegrator{store: inner},
		onEvent: func(e OrchestratorEvent) {
			select {
			case events <- e:
			default:
			}
		},
		logger: slog.Default(),
		active: make(map[string]*activeAgent),
		done:   make(chan struct{}),
	}

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	orch.Start(testCtx)
	defer orch.Stop()

	// Should see transient error events.
	waitForEvent(t, events, "store.transient_error", 3*time.Second)

	// Eventually should recover and spawn a worker.
	waitFor(t, 5*time.Second, "worker spawned after recovery", func() bool {
		return spawner.workerCount() >= 1
	})

	if orch.ConsecutiveStoreFailures() != 0 {
		t.Fatalf("consecutive failures = %d, want 0 after recovery", orch.ConsecutiveStoreFailures())
	}
}

func TestOrchestratorPermanentStoreErrorCountsAsFailure(t *testing.T) {
	inner := newMemoryStore()
	ctx := context.Background()
	now := time.Now().UTC()

	inner.CreateMission(ctx, &Mission{
		ID:        "m1",
		Status:    MissionRunning,
		Budget:    Budget{MaxConcurrentWorkers: 2},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	})

	// Permanent error — no retries, but still counts toward consecutive failures.
	store := newFailingStore(inner, 9999, fmt.Errorf("Error 1146: Table 'missions' doesn't exist"))

	spawner := &mockSpawner{}
	events := make(chan OrchestratorEvent, 100)
	orch := &Orchestrator{
		cfg: OrchestratorConfig{
			MissionID:                   "m1",
			RepoRoot:                    "/test/repo",
			TickInterval:                20 * time.Millisecond,
			MaxAttempts:                 3,
			HeartbeatInterval:           time.Minute,
			StoreRetryAttempts:          3,
			StoreRetryBaseDelay:         time.Millisecond,
			MaxConsecutiveStoreFailures: 3,
		},
		store:   store,
		spawner: spawner,
		workers: &storeWorkerDispatcher{store: inner},
		reviews: &storeReviewDispatcher{store: inner},
		integr:  &storeIntegrator{store: inner},
		onEvent: func(e OrchestratorEvent) {
			select {
			case events <- e:
			default:
			}
		},
		logger: slog.Default(),
		active: make(map[string]*activeAgent),
		done:   make(chan struct{}),
	}

	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	orch.Start(testCtx)

	// Should emit store.error (permanent, not transient).
	waitForEvent(t, events, "store.error", 3*time.Second)

	// Should eventually hit the failure threshold and exit.
	waitForEvent(t, events, "mission.store_failure", 5*time.Second)

	select {
	case <-orch.done:
	case <-time.After(5 * time.Second):
		t.Fatal("orchestrator did not exit after sustained permanent errors")
	}
}
