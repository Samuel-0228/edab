package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

const (
	OnErrorFail     = "fail"
	OnErrorContinue = "continue"
	OnErrorSkipRest = "skip_rest"

	StepTypeAction   = "action"
	StepTypeBranch   = "branch"
	StepTypeForEach  = "for_each"
	StepTypeDelay    = "delay"
	StepTypeWait     = "wait"
	StepTypeParallel = "parallel"
)

// BranchCase defines a conditional branch inside a branch step.
type BranchCase struct {
	When  *conditions.Condition `json:"when"`
	Steps []ActionStep          `json:"steps"`
}

// ActionStep represents an individual action task or control flow block to run within a workflow.
type ActionStep struct {
	ID           string          `json:"id"`
	Name         string          `json:"name,omitempty"`
	Type         string          `json:"type,omitempty"`          // "action" (default), "branch", "for_each", "delay", "parallel"
	Integration  string          `json:"integration,omitempty"`   // "telegram", "discord", "github", "http"
	ActionType   string          `json:"action_type,omitempty"`   // e.g. "send_message", "create_issue", "http_request"
	CredentialID string          `json:"credential_id,omitempty"` // encrypted credentials to inject
	Config       json.RawMessage `json:"config,omitempty"`        // action config (with template variables)
	TimeoutSec   int             `json:"timeout_seconds,omitempty"`
	OnError      string          `json:"on_error,omitempty"`    // "fail" (default), "continue", "skip_rest"
	MaxRetries   *int            `json:"max_retries,omitempty"` // per-step retry override
	BackoffSec   int             `json:"backoff_seconds,omitempty"`

	// Control Flow: Branching
	Cases   []BranchCase `json:"cases,omitempty"`
	Default []ActionStep `json:"default,omitempty"`

	// Control Flow: ForEach
	Items         string `json:"items,omitempty"`
	ItemVar       string `json:"item_var,omitempty"`
	IndexVar      string `json:"index_var,omitempty"`
	MaxIterations int    `json:"max_iterations,omitempty"`

	// Control Flow: Parallel / Fan-out / Inner steps
	Steps       []ActionStep `json:"steps,omitempty"`
	Concurrency int          `json:"concurrency,omitempty"`

	// Control Flow: Delay / Wait
	Duration    string `json:"duration,omitempty"`
	DurationSec int    `json:"duration_seconds,omitempty"`
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
		if err := validateStep(act, fmt.Sprintf("action[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

func validateStep(act ActionStep, prefix string) error {
	switch act.Type {
	case StepTypeBranch:
		if len(act.Cases) == 0 && len(act.Default) == 0 {
			return fmt.Errorf("%s (branch) must define at least one case or default", prefix)
		}
		for cIdx, c := range act.Cases {
			for sIdx, sub := range c.Steps {
				if err := validateStep(sub, fmt.Sprintf("%s.case[%d].step[%d]", prefix, cIdx, sIdx)); err != nil {
					return err
				}
			}
		}
		for sIdx, sub := range act.Default {
			if err := validateStep(sub, fmt.Sprintf("%s.default[%d]", prefix, sIdx)); err != nil {
				return err
			}
		}
	case StepTypeForEach:
		if strings.TrimSpace(act.Items) == "" {
			return fmt.Errorf("%s (for_each) requires 'items' expression", prefix)
		}
		if len(act.Steps) == 0 {
			return fmt.Errorf("%s (for_each) must define at least one child step", prefix)
		}
		for sIdx, sub := range act.Steps {
			if err := validateStep(sub, fmt.Sprintf("%s.step[%d]", prefix, sIdx)); err != nil {
				return err
			}
		}
	case StepTypeParallel:
		if len(act.Steps) == 0 {
			return fmt.Errorf("%s (parallel) must define at least one child step", prefix)
		}
		for sIdx, sub := range act.Steps {
			if err := validateStep(sub, fmt.Sprintf("%s.step[%d]", prefix, sIdx)); err != nil {
				return err
			}
		}
	case StepTypeDelay, StepTypeWait:
		if strings.TrimSpace(act.Duration) == "" && act.DurationSec <= 0 {
			return fmt.Errorf("%s (delay) requires duration or duration_seconds", prefix)
		}
	default:
		if act.Integration == "" {
			return fmt.Errorf("%s.integration is required", prefix)
		}
		if act.ActionType == "" {
			return fmt.Errorf("%s.action_type is required", prefix)
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
