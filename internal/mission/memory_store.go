package mission

import (
	"context"
	"fmt"
	"sync"
)

// InMemoryStore is a full in-memory implementation of Store.
// It is used as a local fallback when the Dolt-backed mission store
// is unavailable, and for lightweight mission workflows that don't need
// cross-process persistence.
type InMemoryStore struct {
	mu          sync.RWMutex
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
	s.missions[m.ID] = cloneMission(m)
	return nil
}

func (s *InMemoryStore) GetMission(_ context.Context, id string) (*Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.missions[id]
	if !ok {
		return nil, notFoundError("mission", id)
	}
	return cloneMission(m), nil
}

func (s *InMemoryStore) UpdateMission(_ context.Context, m *Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.missions[m.ID]; !ok {
		return notFoundError("mission", m.ID)
	}
	s.missions[m.ID] = cloneMission(m)
	return nil
}

func (s *InMemoryStore) ListMissions(_ context.Context) ([]*Mission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Mission, 0, len(s.missions))
	for _, m := range s.missions {
		out = append(out, cloneMission(m))
	}
	sortMissions(out)
	return out, nil
}

func (s *InMemoryStore) CreateTask(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; ok {
		return fmt.Errorf("task %s already exists", t.ID)
	}
	s.tasks[t.ID] = cloneTask(t)
	return nil
}

func (s *InMemoryStore) GetTask(_ context.Context, id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, notFoundError("task", id)
	}
	return cloneTask(t), nil
}

func (s *InMemoryStore) UpdateTask(_ context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; !ok {
		return notFoundError("task", t.ID)
	}
	s.tasks[t.ID] = cloneTask(t)
	return nil
}

func (s *InMemoryStore) ListTasks(_ context.Context, missionID string) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.MissionID == missionID {
			out = append(out, cloneTask(t))
		}
	}
	sortTasks(out)
	return out, nil
}

func (s *InMemoryStore) AddDependency(_ context.Context, dep TaskDependency) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.deps {
		if existing == dep {
			return nil
		}
	}
	s.deps = append(s.deps, dep)
	sortDependencies(s.deps)
	return nil
}

func (s *InMemoryStore) ListDependencies(_ context.Context, missionID string) ([]TaskDependency, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	taskIDs := make(map[string]bool)
	for _, t := range s.tasks {
		if t.MissionID == missionID {
			taskIDs[t.ID] = true
		}
	}
	out := make([]TaskDependency, 0)
	for _, d := range s.deps {
		if taskIDs[d.TaskID] {
			out = append(out, d)
		}
	}
	sortDependencies(out)
	return out, nil
}

func (s *InMemoryStore) CreateRun(_ context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[r.ID]; ok {
		return fmt.Errorf("run %s already exists", r.ID)
	}
	s.runs[r.ID] = cloneRun(r)
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
	s.runs[r.ID] = cloneRun(r)
	return true, nil
}

func (s *InMemoryStore) GetRun(_ context.Context, id string) (*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, notFoundError("run", id)
	}
	return cloneRun(r), nil
}

func (s *InMemoryStore) UpdateRun(_ context.Context, r *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[r.ID]; !ok {
		return notFoundError("run", r.ID)
	}
	s.runs[r.ID] = cloneRun(r)
	return nil
}

func (s *InMemoryStore) ListRuns(_ context.Context, missionID string) ([]*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Run
	for _, r := range s.runs {
		if r.MissionID == missionID {
			out = append(out, cloneRun(r))
		}
	}
	sortRuns(out)
	return out, nil
}

func (s *InMemoryStore) AppendEvent(_ context.Context, e *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextEventID++
	stored := cloneEvent(e)
	stored.ID = s.nextEventID
	s.events = append(s.events, stored)
	e.ID = stored.ID
	return nil
}

func (s *InMemoryStore) ListEvents(_ context.Context, missionID string, limit int) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Event
	for _, e := range s.events {
		if e.MissionID == missionID {
			out = append(out, cloneEvent(e))
		}
	}
	sortEvents(out)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (s *InMemoryStore) CreateArtifact(_ context.Context, a *Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts = append(s.artifacts, cloneArtifact(a))
	sortArtifacts(s.artifacts)
	return nil
}

func (s *InMemoryStore) ListArtifacts(_ context.Context, missionID string) ([]*Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Artifact
	for _, a := range s.artifacts {
		if a.MissionID == missionID {
			out = append(out, cloneArtifact(a))
		}
	}
	sortArtifacts(out)
	return out, nil
}

func (s *InMemoryStore) CreateApproval(_ context.Context, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.approvals[a.ID]; ok {
		return fmt.Errorf("approval %s already exists", a.ID)
	}
	s.approvals[a.ID] = cloneApproval(a)
	return nil
}

func (s *InMemoryStore) GetApproval(_ context.Context, id string) (*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.approvals[id]
	if !ok {
		return nil, notFoundError("approval", id)
	}
	return cloneApproval(a), nil
}

func (s *InMemoryStore) UpdateApproval(_ context.Context, a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.approvals[a.ID]; !ok {
		return notFoundError("approval", a.ID)
	}
	s.approvals[a.ID] = cloneApproval(a)
	return nil
}

func (s *InMemoryStore) ListApprovals(_ context.Context, missionID string) ([]*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Approval
	for _, a := range s.approvals {
		if a.MissionID == missionID {
			out = append(out, cloneApproval(a))
		}
	}
	sortApprovals(out)
	return out, nil
}

func (s *InMemoryStore) GetMissionSummary(ctx context.Context, missionID string) (*MissionSummary, error) {
	return BuildMissionSummary(ctx, s, missionID)
}

func (s *InMemoryStore) GetReadyTasks(_ context.Context, missionID string) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.MissionID == missionID && t.Status == TaskReady {
			out = append(out, cloneTask(t))
		}
	}
	sortTasks(out)
	return out, nil
}

func (s *InMemoryStore) GetTasksByStatus(_ context.Context, missionID string, status TaskStatus) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.MissionID == missionID && t.Status == status {
			out = append(out, cloneTask(t))
		}
	}
	sortTasks(out)
	return out, nil
}

func (s *InMemoryStore) GetRunsForTask(_ context.Context, taskID string) ([]*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Run
	for _, r := range s.runs {
		if r.TaskID == taskID {
			out = append(out, cloneRun(r))
		}
	}
	sortRuns(out)
	return out, nil
}

func (s *InMemoryStore) Close() error { return nil }
