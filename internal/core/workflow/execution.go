package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrExecutionNotFound = errors.New("execution not found")
)

// ExecutionStatus defines the lifecycle status of a workflow execution run.
type ExecutionStatus string

const (
	ExecutionPending   ExecutionStatus = "pending"
	ExecutionRunning   ExecutionStatus = "running"
	ExecutionSuccess   ExecutionStatus = "success"
	ExecutionFailed    ExecutionStatus = "failed"
	ExecutionRetrying  ExecutionStatus = "retrying"
	ExecutionWaiting   ExecutionStatus = "waiting"
	ExecutionCancelled ExecutionStatus = "cancelled"
)

// ExecutionStep represents a granular recorded stage of the execution pipeline:
// trigger -> condition -> transformation -> action.
type ExecutionStep struct {
	ID          string    `json:"id"`
	ExecutionID string    `json:"execution_id"`
	StepID      string    `json:"step_id,omitempty"`
	StepType    string    `json:"step_type"` // "trigger", "condition", "transformation", "action"
	Name        string    `json:"name"`
	Status      string    `json:"status"` // "success", "skipped", "failed"
	Input       any       `json:"input,omitempty"`
	Output      any       `json:"output,omitempty"`
	Error       *string   `json:"error,omitempty"`
	DurationMs  int64     `json:"duration_ms"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// Execution represents an end-to-end execution of a workflow against an event.
type Execution struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id,omitempty"`
	ProjectID      *string         `json:"project_id,omitempty"`
	WorkflowID     string          `json:"workflow_id"`
	EventID        string          `json:"event_id"`
	Status         ExecutionStatus `json:"status"`
	CurrentStepID  string          `json:"current_step_id,omitempty"`
	Priority       int             `json:"priority,omitempty"`
	ConcurrencyKey string          `json:"concurrency_key,omitempty"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	NextAttemptAt  time.Time       `json:"next_attempt_at"`
	StartedAt      time.Time       `json:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	DurationMs     int64           `json:"duration_ms"`
	Error          *string         `json:"error,omitempty"`
	LockedBy       *string         `json:"locked_by,omitempty"`
	LeaseExpiresAt *time.Time      `json:"lease_expires_at,omitempty"`
	Steps          []ExecutionStep `json:"steps,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// ExecutionFilter specifies query options for executions.
type ExecutionFilter struct {
	WorkflowID string
	Status     ExecutionStatus
	Limit      int
	Offset     int
}

// ExecutionStore defines persistence operations for executions and their step history.
type ExecutionStore interface {
	Create(ctx context.Context, exec *Execution) (*Execution, error)
	GetByID(ctx context.Context, id string) (*Execution, error)
	List(ctx context.Context, filter ExecutionFilter) ([]*Execution, error)
	ClaimPending(ctx context.Context) (*Execution, error)
	ClaimPendingWithLease(ctx context.Context, workerID string, leaseDuration time.Duration) (*Execution, error)
	RenewLease(ctx context.Context, id string, workerID string, leaseDuration time.Duration) error
	ReleaseLease(ctx context.Context, id string, workerID string) error
	UpdateStatus(ctx context.Context, id string, status ExecutionStatus, errStr *string, durationMs int64) error
	RecordStep(ctx context.Context, step *ExecutionStep) error
	ResetForRetry(ctx context.Context, id string) (*Execution, error)
	GetByWorkflowAndEvent(ctx context.Context, workflowID, eventID string) (*Execution, error)
}

// MemoryExecutionStore implements ExecutionStore in-memory.
type MemoryExecutionStore struct {
	mu         sync.RWMutex
	executions map[string]*Execution
	steps      map[string][]ExecutionStep
	lookupKey  map[string]string // "workflowID:eventID" -> executionID
}

func NewMemoryExecutionStore() *MemoryExecutionStore {
	return &MemoryExecutionStore{
		executions: make(map[string]*Execution),
		steps:      make(map[string][]ExecutionStep),
		lookupKey:  make(map[string]string),
	}
}

func (s *MemoryExecutionStore) Create(ctx context.Context, exec *Execution) (*Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", exec.WorkflowID, exec.EventID)
	if existingID, exists := s.lookupKey[key]; exists {
		// Idempotency: return existing execution
		return s.executions[existingID], nil
	}

	if exec.ID == "" {
		exec.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	exec.CreatedAt = now
	exec.UpdatedAt = now
	exec.StartedAt = now
	if exec.Status == "" {
		exec.Status = ExecutionPending
	}
	if exec.MaxAttempts <= 0 {
		exec.MaxAttempts = 3
	}

	s.executions[exec.ID] = exec
	s.lookupKey[key] = exec.ID
	return exec, nil
}

func (s *MemoryExecutionStore) GetByID(ctx context.Context, id string) (*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	exec, ok := s.executions[id]
	if !ok {
		return nil, ErrExecutionNotFound
	}
	execCopy := *exec
	if steps, hasSteps := s.steps[id]; hasSteps {
		execCopy.Steps = make([]ExecutionStep, len(steps))
		copy(execCopy.Steps, steps)
	}
	return &execCopy, nil
}

func (s *MemoryExecutionStore) GetByWorkflowAndEvent(ctx context.Context, workflowID, eventID string) (*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", workflowID, eventID)
	id, ok := s.lookupKey[key]
	if !ok {
		return nil, ErrExecutionNotFound
	}
	exec := s.executions[id]
	execCopy := *exec
	if steps, hasSteps := s.steps[id]; hasSteps {
		execCopy.Steps = make([]ExecutionStep, len(steps))
		copy(execCopy.Steps, steps)
	}
	return &execCopy, nil
}

func (s *MemoryExecutionStore) List(ctx context.Context, filter ExecutionFilter) ([]*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Execution
	for _, exec := range s.executions {
		if filter.WorkflowID != "" && exec.WorkflowID != filter.WorkflowID {
			continue
		}
		if filter.Status != "" && exec.Status != filter.Status {
			continue
		}
		execCopy := *exec
		if steps, hasSteps := s.steps[exec.ID]; hasSteps {
			execCopy.Steps = make([]ExecutionStep, len(steps))
			copy(execCopy.Steps, steps)
		}
		result = append(result, &execCopy)
	}

	// Apply limit and offset
	if filter.Offset > 0 {
		if filter.Offset >= len(result) {
			return []*Execution{}, nil
		}
		result = result[filter.Offset:]
	}
	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result, nil
}

func (s *MemoryExecutionStore) ClaimPending(ctx context.Context) (*Execution, error) {
	return s.ClaimPendingWithLease(ctx, "default-worker", 30*time.Second)
}

func (s *MemoryExecutionStore) ClaimPendingWithLease(ctx context.Context, workerID string, leaseDuration time.Duration) (*Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	now := time.Now().UTC()
	var best *Execution
	for _, exec := range s.executions {
		isReady := (exec.Status == ExecutionPending || exec.Status == ExecutionRetrying || exec.Status == ExecutionWaiting) && !exec.NextAttemptAt.After(now)
		isExpired := exec.Status == ExecutionRunning && exec.LeaseExpiresAt != nil && !exec.LeaseExpiresAt.After(now)

		if isReady || isExpired {
			if best == nil || exec.Priority > best.Priority || (exec.Priority == best.Priority && exec.CreatedAt.Before(best.CreatedAt)) {
				best = exec
			}
		}
	}
	if best != nil {
		best.Status = ExecutionRunning
		best.LockedBy = &workerID
		exp := now.Add(leaseDuration)
		best.LeaseExpiresAt = &exp
		best.UpdatedAt = now
		execCopy := *best
		if steps, hasSteps := s.steps[best.ID]; hasSteps {
			execCopy.Steps = make([]ExecutionStep, len(steps))
			copy(execCopy.Steps, steps)
		}
		return &execCopy, nil
	}
	return nil, nil
}

func (s *MemoryExecutionStore) ExpireLeaseForTest(id string, expiry time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if exec, ok := s.executions[id]; ok {
		exec.LeaseExpiresAt = &expiry
	}
}

func (s *MemoryExecutionStore) SetNextAttemptAtForTest(id string, nextAttempt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if exec, ok := s.executions[id]; ok {
		exec.NextAttemptAt = nextAttempt
	}
}

func (s *MemoryExecutionStore) RenewLease(ctx context.Context, id string, workerID string, leaseDuration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	exec, ok := s.executions[id]
	if !ok {
		return ErrExecutionNotFound
	}
	if exec.Status != ExecutionRunning || exec.LockedBy == nil || *exec.LockedBy != workerID {
		return errors.New("execution lease lost or execution not running")
	}

	exp := time.Now().UTC().Add(leaseDuration)
	exec.LeaseExpiresAt = &exp
	exec.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *MemoryExecutionStore) ReleaseLease(ctx context.Context, id string, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	exec, ok := s.executions[id]
	if !ok {
		return ErrExecutionNotFound
	}
	if exec.LockedBy != nil && *exec.LockedBy == workerID && exec.Status == ExecutionRunning {
		exec.LockedBy = nil
		exec.LeaseExpiresAt = nil
		exec.Status = ExecutionPending
		exec.UpdatedAt = time.Now().UTC()
	}
	return nil
}

func (s *MemoryExecutionStore) UpdateStatus(ctx context.Context, id string, status ExecutionStatus, errStr *string, durationMs int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	exec, ok := s.executions[id]
	if !ok {
		return ErrExecutionNotFound
	}

	now := time.Now().UTC()
	exec.Status = status
	exec.UpdatedAt = now
	exec.DurationMs = durationMs
	exec.Error = errStr

	if status == ExecutionSuccess || status == ExecutionFailed || status == ExecutionCancelled {
		exec.CompletedAt = &now
		exec.LockedBy = nil
		exec.LeaseExpiresAt = nil
	} else if status == ExecutionWaiting || status == ExecutionRetrying {
		exec.LockedBy = nil
		exec.LeaseExpiresAt = nil
	}

	return nil
}

func (s *MemoryExecutionStore) RecordStep(ctx context.Context, step *ExecutionStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if step.ID == "" {
		step.ID = uuid.NewString()
	}
	s.steps[step.ExecutionID] = append(s.steps[step.ExecutionID], *step)
	return nil
}

func (s *MemoryExecutionStore) ResetForRetry(ctx context.Context, id string) (*Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	exec, ok := s.executions[id]
	if !ok {
		return nil, ErrExecutionNotFound
	}

	now := time.Now().UTC()
	exec.Status = ExecutionPending
	exec.NextAttemptAt = now
	exec.Error = nil
	exec.LockedBy = nil
	exec.LeaseExpiresAt = nil
	exec.UpdatedAt = now

	execCopy := *exec
	if steps, hasSteps := s.steps[id]; hasSteps {
		execCopy.Steps = make([]ExecutionStep, len(steps))
		copy(execCopy.Steps, steps)
	}
	return &execCopy, nil
}
