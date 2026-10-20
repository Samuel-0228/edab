package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"kerob/internal/core/event"
)

// ActionRequest contains all context required to execute an action step.
type ActionRequest struct {
	WorkflowID  string            `json:"workflow_id"`
	ActionID    string            `json:"action_id"`
	Event       *event.Event      `json:"event"`
	Config      json.RawMessage   `json:"config"`
	Payload     map[string]any    `json:"payload"`
	Credentials map[string]string `json:"-"` // Never serialized
}

// ActionResult holds the result of executing an action.
type ActionResult struct {
	Success    bool            `json:"success"`
	ExternalID string          `json:"external_id,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	Metadata   map[string]any  `json:"metadata,omitempty"`
}

// ActionExecutor represents an action integration handler (e.g. Telegram, Discord, HTTP).
type ActionExecutor interface {
	Name() string
	Type() string
	Validate(config json.RawMessage) error
	Execute(ctx context.Context, req ActionRequest) (ActionResult, error)
}

// ActionRegistry manages registered action executors.
type ActionRegistry struct {
	mu        sync.RWMutex
	executors map[string]ActionExecutor
}

func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{
		executors: make(map[string]ActionExecutor),
	}
}

func (r *ActionRegistry) Register(executor ActionExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[executor.Type()] = executor
}

func (r *ActionRegistry) Get(actionType string) (ActionExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	exec, ok := r.executors[actionType]
	if !ok {
		return nil, fmt.Errorf("no action executor registered for type: %s", actionType)
	}
	return exec, nil
}

func (r *ActionRegistry) RegisteredTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.executors))
	for k := range r.executors {
		types = append(types, k)
	}
	return types
}
