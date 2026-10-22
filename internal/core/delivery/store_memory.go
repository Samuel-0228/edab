package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore provides a thread-safe in-memory implementation of delivery.Store.
type MemoryStore struct {
	mu   sync.RWMutex
	jobs map[string]*DeliveryJob
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		jobs: make(map[string]*DeliveryJob),
	}
}

func (m *MemoryStore) CreateJob(ctx context.Context, job *DeliveryJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
	if job.NextAttemptAt.IsZero() {
		job.NextAttemptAt = now
	}
	if job.Status == "" {
		job.Status = StatusPending
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 3
	}

	cloned := *job
	m.jobs[job.ID] = &cloned
	return nil
}

func (m *MemoryStore) CreateJobs(ctx context.Context, jobs []*DeliveryJob) error {
	for _, j := range jobs {
		if err := m.CreateJob(ctx, j); err != nil {
			return err
		}
	}
	return nil
}

func (m *MemoryStore) GetByID(ctx context.Context, id string) (*DeliveryJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	job, ok := m.jobs[id]
	if !ok {
		return nil, errors.New("delivery job not found")
	}
	cloned := *job
	return &cloned, nil
}

func (m *MemoryStore) List(ctx context.Context, filter ListFilter) ([]*DeliveryJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*DeliveryJob
	for _, j := range m.jobs {
		if filter.EventID != "" && j.EventID != filter.EventID {
			continue
		}
		if filter.DestinationID != "" && j.DestinationID != filter.DestinationID {
			continue
		}
		if filter.Status != "" && j.Status != filter.Status {
			continue
		}
		cloned := *j
		result = append(result, &cloned)
	}

	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	} else if filter.Offset >= len(result) && filter.Offset > 0 {
		return []*DeliveryJob{}, nil
	}

	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}

	return result, nil
}

func (m *MemoryStore) ClaimNextJob(ctx context.Context) (*DeliveryJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	for _, j := range m.jobs {
		if (j.Status == StatusPending || j.Status == StatusFailed) && !j.NextAttemptAt.After(now) {
			j.Status = StatusProcessing
			j.Attempts++
			j.UpdatedAt = now
			cloned := *j
			return &cloned, nil
		}
	}
	return nil, nil // No jobs available
}

func (m *MemoryStore) MarkDelivered(ctx context.Context, id string) error {
	return m.MarkDeliveredWithResult(ctx, id, nil)
}

func (m *MemoryStore) MarkDeliveredWithResult(ctx context.Context, id string, result json.RawMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return errors.New("delivery job not found")
	}

	now := time.Now().UTC()
	j.Status = StatusDelivered
	j.DeliveredAt = &now
	j.UpdatedAt = now
	j.Result = result
	return nil
}

func (m *MemoryStore) Stats(ctx context.Context) (Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var s Stats
	for _, j := range m.jobs {
		s.Total++
		switch j.Status {
		case StatusPending:
			s.Pending++
		case StatusProcessing:
			s.Processing++
		case StatusDelivered:
			s.Delivered++
		case StatusFailed:
			s.Failed++
		case StatusDeadLetter:
			s.DeadLetter++
		}
	}
	return s, nil
}

func (m *MemoryStore) MarkFailed(ctx context.Context, id string, errStr string, nextAttempt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return errors.New("delivery job not found")
	}

	j.Status = StatusFailed
	j.LastError = &errStr
	j.NextAttemptAt = nextAttempt
	j.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryStore) MarkDeadLetter(ctx context.Context, id string, errStr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return errors.New("delivery job not found")
	}

	j.Status = StatusDeadLetter
	j.LastError = &errStr
	j.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryStore) ResetForRetry(ctx context.Context, id string) (*DeliveryJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return nil, errors.New("delivery job not found")
	}

	now := time.Now().UTC()
	j.Status = StatusPending
	j.Attempts = 0
	j.LastError = nil
	j.NextAttemptAt = now
	j.UpdatedAt = now

	cloned := *j
	return &cloned, nil
}
