package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrDeadLetterNotFound = errors.New("dead-letter failure record not found")
)

// DeadLetterRecord stores unrecoverable or exhausted workflow failures.
// Allows operators to inspect errors, understand the context, retry safely, or discard.
type DeadLetterRecord struct {
	ID          string          `json:"id"`
	ExecutionID string          `json:"execution_id"`
	WorkflowID  string          `json:"workflow_id"`
	EventID     string          `json:"event_id"`
	ActionID    string          `json:"action_id,omitempty"`
	Error       string          `json:"error"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Attempts    int             `json:"attempts"`
	Discarded   bool            `json:"discarded"`
	FailedAt    time.Time       `json:"failed_at"`
}

// DeadLetterStore manages dead-letter records.
type DeadLetterStore interface {
	Record(ctx context.Context, rec *DeadLetterRecord) (*DeadLetterRecord, error)
	GetByID(ctx context.Context, id string) (*DeadLetterRecord, error)
	List(ctx context.Context, includeDiscarded bool) ([]*DeadLetterRecord, error)
	Discard(ctx context.Context, id string) error
}

// MemoryDeadLetterStore implements DeadLetterStore in-memory.
type MemoryDeadLetterStore struct {
	mu      sync.RWMutex
	records map[string]*DeadLetterRecord
}

func NewMemoryDeadLetterStore() *MemoryDeadLetterStore {
	return &MemoryDeadLetterStore{
		records: make(map[string]*DeadLetterRecord),
	}
}

func (s *MemoryDeadLetterStore) Record(ctx context.Context, rec *DeadLetterRecord) (*DeadLetterRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec.ID == "" {
		rec.ID = uuid.NewString()
	}
	if rec.FailedAt.IsZero() {
		rec.FailedAt = time.Now().UTC()
	}

	s.records[rec.ID] = rec
	return rec, nil
}

func (s *MemoryDeadLetterStore) GetByID(ctx context.Context, id string) (*DeadLetterRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.records[id]
	if !ok {
		return nil, ErrDeadLetterNotFound
	}
	return rec, nil
}

func (s *MemoryDeadLetterStore) List(ctx context.Context, includeDiscarded bool) ([]*DeadLetterRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*DeadLetterRecord, 0)
	for _, rec := range s.records {
		if !includeDiscarded && rec.Discarded {
			continue
		}
		list = append(list, rec)
	}
	return list, nil
}

func (s *MemoryDeadLetterStore) Discard(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[id]
	if !ok {
		return ErrDeadLetterNotFound
	}
	rec.Discarded = true
	return nil
}
