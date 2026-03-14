package mission

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// InMemoryStore is a full in-memory implementation of Store.
// It is used as a local fallback when the Dolt-backed mission store
// is unavailable, and for lightweight mission workflows that don't need
// cross-process persistence.
type InMemoryStore struct {
	mu          sync.Mutex
	missions    map[string]*Mission
	tasks       map[string]*Task
	deps        []TaskDependency
	runs        map[string]*Run
	events      []*Event
	artifacts   []*Artifact
	approvals   map[string]*Approval
	nextEventID int64
}

// NewInMemoryStore creates an empty in-memory mission store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		missions:  make(map[string]*Mission),
		tasks:     make(map[string]*Task),
		runs:      make(map[string]*Run),
		approvals: make(map[string]*Approval),
	}
}

func (s *InMemoryStore) CreateMission(_ context.Context, m *Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.missions[m.ID]; ok {
		return fmt.Errorf("mission %s already exists", m.ID)
	}
	cp := *m
	s.missions[m.ID] = &cp
	return nil
}

func (s *InMemoryStore) GetMission(_ context.Context, id string) (*Mission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.missions[id]
	if !ok {
		return nil, fmt.Errorf("mission %s not found", id)
	}
	cp := *m
	return &cp, nil
}

func (s *InMemoryStore) UpdateMission(_ context.Context, m *Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.missions[m.ID]; !ok {
		return fmt.Errorf("mission %s not found", m.ID)
	}
	cp := *m
	s.missions[m.ID] = &cp
	return nil
}

func (s *InMemoryStore) ListMissions(_ context.Context) ([]*Mission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Mission, 0, len(s.missions))
	for _, m := range s.missions {
		cp := *m
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *InMemoryStore) CreateTask(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; ok {
		return fmt.Errorf("task %s already exists", t.ID)
	}
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *InMemoryStore) GetTask(_ context.Context, id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found", id)
	}
	cp := *t
	return &cp, nil
}

func (s *InMemoryStore) UpdateTask(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; !ok {
		return fmt.Errorf("task %s not found", t.ID)
	}
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *InMemoryStore) ListTasks(_ context.Context, missionID string) ([]*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.MissionID == missionID {
			cp := *t
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *InMemoryStore) AddDependency(_ context.Context, dep TaskDependency) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deps = append(s.deps, dep)
	return nil
}

func (s *InMemoryStore) ListDependencies(_ context.Context, missionID string) ([]TaskDependency, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	taskIDs := make(map[string]bool)
	for _, t := range s.tasks {
		if t.MissionID == missionID {
			taskIDs[t.ID] = true
		}
	}
	var out []TaskDependency
	for _, d := range s.deps {
		if taskIDs[d.TaskID] || taskIDs[d.DependsOnID] {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *InMemoryStore) CreateRun(_ context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[r.ID]; ok {
		return fmt.Errorf("run %s already exists", r.ID)
	}
	cp := *r
	s.runs[r.ID] = &cp
	return nil
}

func (s *InMemoryStore) CreateRunExclusive(_ context.Context, r *Run) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runs {
		if existing.TaskID == r.TaskID && existing.Mode == r.Mode && existing.Status == RunRunning {
			return false, nil
		}
	}
	if _, ok := s.runs[r.ID]; ok {
		return false, fmt.Errorf("run %s already exists", r.ID)
	}
	cp := *r
	s.runs[r.ID] = &cp
	return true, nil
}

func (s *InMemoryStore) GetRun(_ context.Context, id string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, fmt.Errorf("run %s not found", id)
	}
	cp := *r
	return &cp, nil
}

func (s *InMemoryStore) UpdateRun(_ context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[r.ID]; !ok {
		return fmt.Errorf("run %s not found", r.ID)
	}
	cp := *r
	s.runs[r.ID] = &cp
	return nil
}

func (s *InMemoryStore) ListRuns(_ context.Context, missionID string) ([]*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Run
	for _, r := range s.runs {
		if r.MissionID == missionID {
			cp := *r
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		var left, right string
		if out[i].StartedAt != nil {
			left = formatTime(*out[i].StartedAt)
		}
		if out[j].StartedAt != nil {
			right = formatTime(*out[j].StartedAt)
		}
		return left > right
	})
	return out, nil
}

func (s *InMemoryStore) AppendEvent(_ context.Context, e *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextEventID++
	e.ID = s.nextEventID
	cp := *e
	s.events = append(s.events, &cp)
	return nil
}

func (s *InMemoryStore) ListEvents(_ context.Context, missionID string, limit int) ([]*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Event
	for _, e := range s.events {
		if e.MissionID == missionID {
			cp := *e
			out = append(out, &cp)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (s *InMemoryStore) CreateArtifact(_ context.Context, a *Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *a
	s.artifacts = append(s.artifacts, &cp)
	return nil
}

func (s *InMemoryStore) ListArtifacts(_ context.Context, missionID string) ([]*Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Artifact
	for _, a := range s.artifacts {
		if a.MissionID == missionID {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *InMemoryStore) CreateApproval(_ context.Context, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.approvals[a.ID]; ok {
		return fmt.Errorf("approval %s already exists", a.ID)
	}
	cp := *a
	s.approvals[a.ID] = &cp
	return nil
}

func (s *InMemoryStore) GetApproval(_ context.Context, id string) (*Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.approvals[id]
	if !ok {
		return nil, fmt.Errorf("approval %s not found", id)
	}
	cp := *a
	return &cp, nil
}

func (s *InMemoryStore) UpdateApproval(_ context.Context, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.approvals[a.ID]; !ok {
		return fmt.Errorf("approval %s not found", a.ID)
	}
	cp := *a
	s.approvals[a.ID] = &cp
	return nil
}

func (s *InMemoryStore) ListApprovals(_ context.Context, missionID string) ([]*Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Approval
	for _, a := range s.approvals {
		if a.MissionID == missionID {
			cp := *a
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *InMemoryStore) GetMissionSummary(_ context.Context, missionID string) (*MissionSummary, error) {
	s.mu.Lock()
	m, ok := s.missions[missionID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("mission %s not found", missionID)
	}
	mCopy := *m

	var counts TaskCounts
	for _, t := range s.tasks {
		if t.MissionID != missionID {
			continue
		}
		counts.Total++
		switch t.Status {
		case TaskPending:
			counts.Pending++
		case TaskReady:
			counts.Ready++
		case TaskRunning, TaskLeased:
			counts.Running++
		case TaskAwaitingReview:
			counts.AwaitingReview++
		case TaskAccepted:
			counts.Accepted++
		case TaskIntegrated:
			counts.Integrated++
		case TaskDone:
			counts.Done++
		case TaskBlocked:
			counts.Blocked++
		case TaskFailed, TaskRejected:
			counts.Failed++
		}
	}

	var activeRuns int
	for _, r := range s.runs {
		if r.MissionID == missionID && r.Status == RunRunning {
			activeRuns++
		}
	}

	var pendingApprovals int
	for _, a := range s.approvals {
		if a.MissionID == missionID && a.Status == ApprovalPending {
			pendingApprovals++
		}
	}
	s.mu.Unlock()

	return &MissionSummary{
		Mission:          &mCopy,
		TaskCounts:       counts,
		ActiveRuns:       activeRuns,
		PendingApprovals: pendingApprovals,
	}, nil
}

func (s *InMemoryStore) GetReadyTasks(_ context.Context, missionID string) ([]*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.MissionID == missionID && t.Status == TaskReady {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *InMemoryStore) GetTasksByStatus(_ context.Context, missionID string, status TaskStatus) ([]*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.MissionID == missionID && t.Status == status {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *InMemoryStore) GetRunsForTask(_ context.Context, taskID string) ([]*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Run
	for _, r := range s.runs {
		if r.TaskID == taskID {
			cp := *r
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *InMemoryStore) Close() error { return nil }
