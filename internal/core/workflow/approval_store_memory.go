package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryApprovalStore implements ApprovalStore in-memory for testing and local operation.
type MemoryApprovalStore struct {
	mu        sync.RWMutex
	approvals map[string]*Approval
	byToken   map[string]string // tokenHash -> approvalID
	byExec    map[string]string // "execID:stepID" -> approvalID
}

// NewMemoryApprovalStore creates an in-memory approval store.
func NewMemoryApprovalStore() *MemoryApprovalStore {
	return &MemoryApprovalStore{
		approvals: make(map[string]*Approval),
		byToken:   make(map[string]string),
		byExec:    make(map[string]string),
	}
}

func (s *MemoryApprovalStore) Create(ctx context.Context, app *Approval) (*Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	execKey := fmt.Sprintf("%s:%s", app.ExecutionID, app.StepID)
	if existingID, exists := s.byExec[execKey]; exists {
		stored := *s.approvals[existingID]
		return &stored, nil
	}

	if app.ID == "" {
		app.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	app.CreatedAt = now
	app.UpdatedAt = now
	if app.Status == "" {
		app.Status = ApprovalPending
	}
	if app.ExpiresAt.IsZero() {
		app.ExpiresAt = now.Add(24 * time.Hour)
	}

	stored := *app
	s.approvals[app.ID] = &stored
	s.byExec[execKey] = app.ID
	if app.TokenHash != "" {
		s.byToken[app.TokenHash] = app.ID
	}

	ret := stored
	return &ret, nil
}

func (s *MemoryApprovalStore) GetByID(ctx context.Context, id string) (*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	app, ok := s.approvals[id]
	if !ok {
		return nil, ErrApprovalNotFound
	}
	ret := *app
	return &ret, nil
}

func (s *MemoryApprovalStore) GetByTokenHash(ctx context.Context, tokenHash string) (*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byToken[tokenHash]
	if !ok {
		return nil, ErrApprovalNotFound
	}
	app, ok := s.approvals[id]
	if !ok {
		return nil, ErrApprovalNotFound
	}
	ret := *app
	return &ret, nil
}

func (s *MemoryApprovalStore) GetByExecutionAndStep(ctx context.Context, executionID, stepID string) (*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	execKey := fmt.Sprintf("%s:%s", executionID, stepID)
	id, ok := s.byExec[execKey]
	if !ok {
		return nil, ErrApprovalNotFound
	}
	app, ok := s.approvals[id]
	if !ok {
		return nil, ErrApprovalNotFound
	}
	ret := *app
	return &ret, nil
}

func (s *MemoryApprovalStore) List(ctx context.Context, filter ApprovalFilter) ([]*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Approval
	for _, app := range s.approvals {
		if filter.TenantID != "" && app.TenantID != "" && app.TenantID != filter.TenantID {
			continue
		}
		if filter.ExecutionID != "" && app.ExecutionID != filter.ExecutionID {
			continue
		}
		if filter.WorkflowID != "" && app.WorkflowID != filter.WorkflowID {
			continue
		}
		if filter.Status != "" && app.Status != filter.Status {
			continue
		}
		ret := *app
		result = append(result, &ret)
	}

	// Apply pagination
	if filter.Offset > len(result) {
		return []*Approval{}, nil
	}
	result = result[filter.Offset:]
	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result, nil
}

func (s *MemoryApprovalStore) UpdateDecision(ctx context.Context, id string, status ApprovalStatus, decidedBy string, note string) (*Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.approvals[id]
	if !ok {
		return nil, ErrApprovalNotFound
	}
	if app.Status != ApprovalPending && app.Status != ApprovalEscalated {
		return nil, ErrApprovalAlreadyClosed
	}

	app.Status = status
	app.ApprovedBy = &decidedBy
	app.DecisionNote = &note
	app.UpdatedAt = time.Now().UTC()

	ret := *app
	return &ret, nil
}

func (s *MemoryApprovalStore) UpdateStatus(ctx context.Context, id string, status ApprovalStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.approvals[id]
	if !ok {
		return ErrApprovalNotFound
	}
	app.Status = status
	app.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *MemoryApprovalStore) ListExpiredPending(ctx context.Context, now time.Time) ([]*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var expired []*Approval
	for _, app := range s.approvals {
		if (app.Status == ApprovalPending || app.Status == ApprovalEscalated) && !app.ExpiresAt.IsZero() && now.After(app.ExpiresAt) {
			ret := *app
			expired = append(expired, &ret)
		}
	}
	return expired, nil
}

func (s *MemoryApprovalStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.approvals[id]
	if !ok {
		return nil
	}
	execKey := fmt.Sprintf("%s:%s", app.ExecutionID, app.StepID)
	delete(s.byExec, execKey)
	if app.TokenHash != "" {
		delete(s.byToken, app.TokenHash)
	}
	delete(s.approvals, id)
	return nil
}
