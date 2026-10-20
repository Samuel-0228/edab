package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"kerob/internal/core/conditions"
	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
)

var (
	ErrWorkflowNotFound = errors.New("workflow not found")
)

// TriggerConfig defines the trigger event criteria for activating a workflow.
type TriggerConfig struct {
	Type       string          `json:"type"`                  // "telegram", "webhook", "discord", "rss", "github", "schedule", "all"
	SourceID   string          `json:"source_id,omitempty"`   // optional source restriction
	EventTypes []string        `json:"event_types,omitempty"` // empty or ["*"] matches all event types for trigger
	Config     json.RawMessage `json:"config,omitempty"`      // trigger-specific configuration
}

// Matches checks whether the incoming event matches the trigger criteria.
func (tc *TriggerConfig) Matches(ev *event.Event) bool {
	if tc.SourceID != "" && tc.SourceID != ev.SourceID {
		return false
	}

	if len(tc.EventTypes) > 0 {
		matched := false
		for _, et := range tc.EventTypes {
			if et == "*" || et == ev.EventType {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// ActionStep represents an individual action task to run within a workflow.
type ActionStep struct {
	ID           string          `json:"id"`
	Integration  string          `json:"integration"`             // "telegram", "discord", "github", "http"
	ActionType   string          `json:"action_type"`             // e.g. "send_message", "create_issue", "http_request"
	CredentialID string          `json:"credential_id,omitempty"` // encrypted credentials to inject
	Config       json.RawMessage `json:"config"`                  // action config (with template variables)
	TimeoutSec   int             `json:"timeout_seconds,omitempty"`
}

// Workflow represents a declarative, event-driven automation pipeline.
// Structure: WHEN [trigger] IF [conditions] THEN [actions]
type Workflow struct {
	ID              string                          `json:"id"`
	Name            string                          `json:"name"`
	Description     string                          `json:"description,omitempty"`
	Enabled         bool                            `json:"enabled"`
	Trigger         TriggerConfig                   `json:"trigger"`
	Conditions      *conditions.Condition           `json:"conditions,omitempty"`
	Transformations *transformations.Transformation `json:"transformations,omitempty"`
	Actions         []ActionStep                    `json:"actions"`
	MaxRetries      int                             `json:"max_retries"`
	TimeoutSeconds  int                             `json:"timeout_seconds"`
	CreatedAt       time.Time                       `json:"created_at"`
	UpdatedAt       time.Time                       `json:"updated_at"`
}

// Validate checks the workflow structure and constraints.
func (w *Workflow) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("workflow name is required")
	}
	if w.Trigger.Type == "" {
		return fmt.Errorf("workflow trigger.type is required")
	}
	if len(w.Actions) == 0 {
		return fmt.Errorf("workflow must define at least one action")
	}
	for i, act := range w.Actions {
		if act.Integration == "" {
			return fmt.Errorf("action[%d].integration is required", i)
		}
		if act.ActionType == "" {
			return fmt.Errorf("action[%d].action_type is required", i)
		}
	}
	return nil
}

// Store defines persistent storage operations for Workflows.
type Store interface {
	Create(ctx context.Context, wf *Workflow) (*Workflow, error)
	GetByID(ctx context.Context, id string) (*Workflow, error)
	List(ctx context.Context) ([]*Workflow, error)
	ListActive(ctx context.Context) ([]*Workflow, error)
	Update(ctx context.Context, wf *Workflow) (*Workflow, error)
	Delete(ctx context.Context, id string) error
}

// MemoryStore provides in-memory workflow storage.
type MemoryStore struct {
	workflows map[string]*Workflow
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		workflows: make(map[string]*Workflow),
	}
}

func (s *MemoryStore) Create(ctx context.Context, wf *Workflow) (*Workflow, error) {
	if err := wf.Validate(); err != nil {
		return nil, err
	}
	if wf.ID == "" {
		wf.ID = uuid.NewString()
	}
	if wf.MaxRetries <= 0 {
		wf.MaxRetries = 3
	}
	if wf.TimeoutSeconds <= 0 {
		wf.TimeoutSeconds = 30
	}
	now := time.Now().UTC()
	wf.CreatedAt = now
	wf.UpdatedAt = now

	for i := range wf.Actions {
		if wf.Actions[i].ID == "" {
			wf.Actions[i].ID = uuid.NewString()
		}
	}

	s.workflows[wf.ID] = wf
	return wf, nil
}

func (s *MemoryStore) GetByID(ctx context.Context, id string) (*Workflow, error) {
	wf, ok := s.workflows[id]
	if !ok {
		return nil, ErrWorkflowNotFound
	}
	return wf, nil
}

func (s *MemoryStore) List(ctx context.Context) ([]*Workflow, error) {
	list := make([]*Workflow, 0, len(s.workflows))
	for _, wf := range s.workflows {
		list = append(list, wf)
	}
	return list, nil
}

func (s *MemoryStore) ListActive(ctx context.Context) ([]*Workflow, error) {
	list := make([]*Workflow, 0)
	for _, wf := range s.workflows {
		if wf.Enabled {
			list = append(list, wf)
		}
	}
	return list, nil
}

func (s *MemoryStore) Update(ctx context.Context, wf *Workflow) (*Workflow, error) {
	if err := wf.Validate(); err != nil {
		return nil, err
	}
	existing, ok := s.workflows[wf.ID]
	if !ok {
		return nil, ErrWorkflowNotFound
	}

	wf.UpdatedAt = time.Now().UTC()
	wf.CreatedAt = existing.CreatedAt
	s.workflows[wf.ID] = wf
	return wf, nil
}

func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	if _, ok := s.workflows[id]; !ok {
		return ErrWorkflowNotFound
	}
	delete(s.workflows, id)
	return nil
}
