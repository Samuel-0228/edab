package workflow

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore provides a thread-safe in-memory implementation of Store for testing and offline environments.
type MemoryStore struct {
	mu        sync.RWMutex
	workflows map[string]*Workflow
}

// NewMemoryStore initializes and returns a new thread-safe in-memory workflow store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		workflows: make(map[string]*Workflow),
	}
}

// cloneWorkflow produces a deep copy of a Workflow to prevent concurrent mutations or cross-test contamination.
func cloneWorkflow(wf *Workflow) *Workflow {
	if wf == nil {
		return nil
	}
	cp := *wf

	// Deep copy Trigger
	if wf.Trigger.EventTypes != nil {
		cp.Trigger.EventTypes = make([]string, len(wf.Trigger.EventTypes))
		copy(cp.Trigger.EventTypes, wf.Trigger.EventTypes)
	}
	if len(wf.Trigger.Config) > 0 {
		cp.Trigger.Config = make(json.RawMessage, len(wf.Trigger.Config))
		copy(cp.Trigger.Config, wf.Trigger.Config)
	}

	// Deep copy Actions
	if wf.Actions != nil {
		cp.Actions = make([]ActionStep, len(wf.Actions))
		for i, a := range wf.Actions {
			actionCp := a
			if len(a.Config) > 0 {
				actionCp.Config = make(json.RawMessage, len(a.Config))
				copy(actionCp.Config, a.Config)
			}
			cp.Actions[i] = actionCp
		}
	}

	// Deep copy Conditions & Transformations if needed
	if wf.Conditions != nil {
		condCp := *wf.Conditions
		cp.Conditions = &condCp
	}
	if wf.Transformations != nil {
		transCp := *wf.Transformations
		if wf.Transformations.FieldMaps != nil {
			transCp.FieldMaps = make(map[string]string, len(wf.Transformations.FieldMaps))
			for k, v := range wf.Transformations.FieldMaps {
				transCp.FieldMaps[k] = v
			}
		}
		if wf.Transformations.Defaults != nil {
			transCp.Defaults = make(map[string]any, len(wf.Transformations.Defaults))
			for k, v := range wf.Transformations.Defaults {
				transCp.Defaults[k] = v
			}
		}
		cp.Transformations = &transCp
	}

	return &cp
}

// Create inserts a validated workflow into memory.
func (s *MemoryStore) Create(ctx context.Context, wf *Workflow) (*Workflow, error) {
	if err := wf.Validate(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item := cloneWorkflow(wf)
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if item.MaxRetries <= 0 {
		item.MaxRetries = 3
	}
	if item.TimeoutSeconds <= 0 {
		item.TimeoutSeconds = 30
	}
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now

	for i := range item.Actions {
		if item.Actions[i].ID == "" {
			item.Actions[i].ID = uuid.NewString()
		}
	}

	s.workflows[item.ID] = cloneWorkflow(item)
	return cloneWorkflow(item), nil
}

// GetByID retrieves a workflow by ID from memory.
func (s *MemoryStore) GetByID(ctx context.Context, id string) (*Workflow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	wf, ok := s.workflows[id]
	if !ok {
		return nil, ErrWorkflowNotFound
	}
	return cloneWorkflow(wf), nil
}

// List returns all workflows stored in memory.
func (s *MemoryStore) List(ctx context.Context) ([]*Workflow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Workflow, 0, len(s.workflows))
	for _, wf := range s.workflows {
		list = append(list, cloneWorkflow(wf))
	}
	return list, nil
}

// ListActive returns only enabled workflows stored in memory.
func (s *MemoryStore) ListActive(ctx context.Context) ([]*Workflow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Workflow, 0)
	for _, wf := range s.workflows {
		if wf.Enabled {
			list = append(list, cloneWorkflow(wf))
		}
	}
	return list, nil
}

// Update updates an existing workflow in memory.
func (s *MemoryStore) Update(ctx context.Context, wf *Workflow) (*Workflow, error) {
	if err := wf.Validate(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.workflows[wf.ID]
	if !ok {
		return nil, ErrWorkflowNotFound
	}

	item := cloneWorkflow(wf)
	item.UpdatedAt = time.Now().UTC()
	item.CreatedAt = existing.CreatedAt

	for i := range item.Actions {
		if item.Actions[i].ID == "" {
			item.Actions[i].ID = uuid.NewString()
		}
	}

	s.workflows[item.ID] = cloneWorkflow(item)
	return cloneWorkflow(item), nil
}

// Delete removes a workflow by ID from memory.
func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.workflows[id]; !ok {
		return ErrWorkflowNotFound
	}
	delete(s.workflows, id)
	return nil
}
