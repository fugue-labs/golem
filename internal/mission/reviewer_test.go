package mission

import (
	"context"
	"strings"
	"testing"
	"time"
)

// reviewMockStore extends workerMockStore with methods needed for ReviewLauncher.
type reviewMockStore struct {
	workerMockStore
	approvals  []*Approval
	artifacts  []*Artifact
	tasksByStatus map[TaskStatus][]*Task
	runsByTask map[string][]*Run
}

func newReviewMockStore() *reviewMockStore {
	return &reviewMockStore{
		workerMockStore: workerMockStore{
			mockStore: mockStore{
				missions: make(map[string]*Mission),
			},
			tasks: make(map[string]*Task),
			runs:  make(map[string]*Run),
		},
		tasksByStatus: make(map[TaskStatus][]*Task),
		runsByTask:    make(map[string][]*Run),
	}
}

func (s *reviewMockStore) GetTasksByStatus(_ context.Context, _ string, status TaskStatus) ([]*Task, error) {
	return s.tasksByStatus[status], nil
}

func (s *reviewMockStore) GetRunsForTask(_ context.Context, taskID string) ([]*Run, error) {
	return s.runsByTask[taskID], nil
}

func (s *reviewMockStore) CreateApproval(_ context.Context, a *Approval) error {
	s.approvals = append(s.approvals, a)
	return nil
}

func (s *reviewMockStore) CreateArtifact(_ context.Context, a *Artifact) error {
	s.artifacts = append(s.artifacts, a)
	return nil
}

// --- Tests ---

func TestCompleteReview_Pass(t *testing.T) {
	store := newReviewMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	task := &Task{ID: "t1", MissionID: "m1", Status: TaskAwaitingReview}
	store.tasks["t1"] = task

	now := time.Now().UTC()
	run := &Run{ID: "r_review", MissionID: "m1", TaskID: "t1", Mode: RunModeReview, Status: RunRunning, StartedAt: &now}
	store.runs["r_review"] = run

	launcher := NewReviewLauncher(store)
	spec := &ReviewSpec{Run: run, Task: task}

	err := launcher.CompleteReview(context.Background(), spec, &ReviewResult{
		Verdict: ReviewPass,
		Summary: "Code looks good",
	})
	if err != nil {
		t.Fatal(err)
	}

	if spec.Run.Status != RunSucceeded {
		t.Errorf("expected run status succeeded, got %s", spec.Run.Status)
	}
	if spec.Task.Status != TaskAccepted {
		t.Errorf("expected task status accepted, got %s", spec.Task.Status)
	}

	// Should have an approval record.
	if len(store.approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(store.approvals))
	}
	if store.approvals[0].Status != ApprovalApproved {
		t.Errorf("expected approval status approved, got %s", store.approvals[0].Status)
	}

	// Should have a review.passed event.
	found := false
	for _, e := range store.events {
		if e.Type == "review.passed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected review.passed event")
	}
}

func TestCompleteReview_Reject(t *testing.T) {
	store := newReviewMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	task := &Task{ID: "t1", MissionID: "m1", Status: TaskAwaitingReview, AttemptCount: 1}
	store.tasks["t1"] = task

	now := time.Now().UTC()
	run := &Run{ID: "r_review", MissionID: "m1", TaskID: "t1", Mode: RunModeReview, Status: RunRunning, StartedAt: &now}
	store.runs["r_review"] = run

	launcher := NewReviewLauncher(store)
	spec := &ReviewSpec{Run: run, Task: task}

	err := launcher.CompleteReview(context.Background(), spec, &ReviewResult{
		Verdict: ReviewReject,
		Summary: "Missing error handling",
	})
	if err != nil {
		t.Fatal(err)
	}

	if spec.Task.Status != TaskReady {
		t.Errorf("expected task status ready (retry), got %s", spec.Task.Status)
	}
	if len(store.approvals) != 1 || store.approvals[0].Status != ApprovalRejected {
		t.Error("expected rejected approval")
	}
}

func TestCompleteReview_RequestChanges(t *testing.T) {
	store := newReviewMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	task := &Task{ID: "t1", MissionID: "m1", Status: TaskAwaitingReview}
	store.tasks["t1"] = task

	now := time.Now().UTC()
	run := &Run{ID: "r_review", MissionID: "m1", TaskID: "t1", Mode: RunModeReview, Status: RunRunning, StartedAt: &now}
	store.runs["r_review"] = run

	launcher := NewReviewLauncher(store)
	spec := &ReviewSpec{Run: run, Task: task}

	err := launcher.CompleteReview(context.Background(), spec, &ReviewResult{
		Verdict:    ReviewRequestChanges,
		Summary:    "Need better test coverage",
		Suggestion: "Add unit tests for edge cases",
	})
	if err != nil {
		t.Fatal(err)
	}

	if spec.Task.Status != TaskReady {
		t.Errorf("expected task status ready (auto-retry), got %s", spec.Task.Status)
	}
	if spec.Task.BlockingReason != "Add unit tests for edge cases" {
		t.Errorf("expected blocking reason from suggestion, got %s", spec.Task.BlockingReason)
	}
}

func TestFailReview(t *testing.T) {
	store := newReviewMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	task := &Task{ID: "t1", MissionID: "m1", Status: TaskAwaitingReview}
	store.tasks["t1"] = task

	now := time.Now().UTC()
	run := &Run{ID: "r_review", MissionID: "m1", TaskID: "t1", Mode: RunModeReview, Status: RunRunning, StartedAt: &now}
	store.runs["r_review"] = run

	launcher := NewReviewLauncher(store)
	spec := &ReviewSpec{Run: run, Task: task}

	err := launcher.FailReview(context.Background(), spec, "reviewer timed out")
	if err != nil {
		t.Fatal(err)
	}

	if spec.Run.Status != RunFailed {
		t.Errorf("expected run status failed, got %s", spec.Run.Status)
	}
	// Task should stay in awaiting_review for re-review.
	if task.Status != TaskAwaitingReview {
		t.Errorf("expected task still awaiting_review, got %s", task.Status)
	}
}

func TestDispatchPendingReviews_NoTasks(t *testing.T) {
	store := newReviewMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning}

	launcher := NewReviewLauncher(store)
	specs, err := launcher.DispatchPendingReviews(context.Background(), "m1", "/tmp/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 0 {
		t.Fatalf("expected 0 specs, got %d", len(specs))
	}
}

func TestDispatchPendingReviews_SkipsActiveReview(t *testing.T) {
	store := newReviewMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionRunning, BaseBranch: "main"}

	task := &Task{ID: "t1", MissionID: "m1", Status: TaskAwaitingReview}
	store.tasks["t1"] = task
	store.tasksByStatus[TaskAwaitingReview] = []*Task{task}

	// Active review run already exists.
	activeReview := &Run{ID: "r_active", MissionID: "m1", TaskID: "t1", Mode: RunModeReview, Status: RunRunning}
	store.runsByTask["t1"] = []*Run{activeReview}

	launcher := NewReviewLauncher(store)
	specs, err := launcher.DispatchPendingReviews(context.Background(), "m1", "/tmp/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 0 {
		t.Fatalf("expected 0 specs (review already active), got %d", len(specs))
	}
}

func TestDispatchPendingReviews_NotRunning(t *testing.T) {
	store := newReviewMockStore()
	store.missions["m1"] = &Mission{ID: "m1", Status: MissionPaused}

	launcher := NewReviewLauncher(store)
	_, err := launcher.DispatchPendingReviews(context.Background(), "m1", "/tmp/repo")
	if err == nil {
		t.Fatal("expected error for non-running mission")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildReviewPrompt(t *testing.T) {
	task := &Task{
		ID:        "t1",
		Title:     "Add user auth",
		Kind:      TaskKindCode,
		Objective: "Implement JWT auth middleware.",
		AcceptanceCriteria: []string{
			"JWT tokens validated",
			"401 for unauthorized",
		},
	}

	workerRun := &Run{
		ID:      "r1",
		Summary: "Added JWT middleware to api package",
	}

	diff := `diff --git a/pkg/auth/jwt.go b/pkg/auth/jwt.go
new file mode 100644
+package auth
+func ValidateToken(token string) error {`

	prompt := BuildReviewPrompt(task, workerRun, diff)

	checks := []struct {
		name     string
		contains string
	}{
		{"task ID", "t1"},
		{"title", "Add user auth"},
		{"objective", "JWT auth middleware"},
		{"criteria", "JWT tokens validated"},
		{"criteria 2", "401 for unauthorized"},
		{"worker summary", "Added JWT middleware"},
		{"diff", "+package auth"},
		{"verdict format", "\"verdict\""},
		{"untrusted", "untrusted"},
		{"acceptance criterion check", "acceptance criterion"},
	}

	for _, c := range checks {
		if !strings.Contains(prompt, c.contains) {
			t.Errorf("prompt missing %s (%q)", c.name, c.contains)
		}
	}
}

func TestBuildReviewPrompt_EmptyDiff(t *testing.T) {
	task := &Task{ID: "t1", Title: "No-op", Kind: TaskKindCode, Objective: "Test"}
	prompt := BuildReviewPrompt(task, nil, "")

	if !strings.Contains(prompt, "(no changes)") {
		t.Error("expected (no changes) for empty diff")
	}
}

func TestBuildReviewPrompt_NoWorkerSummary(t *testing.T) {
	task := &Task{ID: "t1", Title: "Test", Kind: TaskKindCode, Objective: "Test"}
	prompt := BuildReviewPrompt(task, nil, "some diff")

	if strings.Contains(prompt, "Worker Summary") {
		t.Error("should not have Worker Summary section when no worker run")
	}
}
