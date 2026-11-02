package streaming

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrDLQNotFound = errors.New("dead-letter event not found")
)

// MemoryOffsetStore stores consumer group offsets in memory.
type MemoryOffsetStore struct {
	mu      sync.RWMutex
	offsets map[string]ConsumerOffset // key: "group:topic:partition"
}

// NewMemoryOffsetStore creates a new MemoryOffsetStore.
func NewMemoryOffsetStore() *MemoryOffsetStore {
	return &MemoryOffsetStore{
		offsets: make(map[string]ConsumerOffset),
	}
}

func (s *MemoryOffsetStore) CommitOffset(ctx context.Context, offset ConsumerOffset) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%d", offset.GroupID, offset.Topic, offset.Partition)
	if offset.CommittedAt.IsZero() {
		offset.CommittedAt = time.Now().UTC()
	}
	s.offsets[key] = offset
	return nil
}

func (s *MemoryOffsetStore) GetOffset(ctx context.Context, groupID, topic string, partition int32) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s:%d", groupID, topic, partition)
	off, ok := s.offsets[key]
	if !ok {
		return 0, nil
	}
	return off.Offset, nil
}

func (s *MemoryOffsetStore) ListOffsets(ctx context.Context, groupID string) ([]ConsumerOffset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []ConsumerOffset
	for _, off := range s.offsets {
		if groupID == "" || off.GroupID == groupID {
			result = append(result, off)
		}
	}
	return result, nil
}

// MemoryDeadLetterStore stores dead-letter events in memory.
type MemoryDeadLetterStore struct {
	mu     sync.RWMutex
	events map[string]*DeadLetterEvent // key: ID
}

// NewMemoryDeadLetterStore creates a new MemoryDeadLetterStore.
func NewMemoryDeadLetterStore() *MemoryDeadLetterStore {
	return &MemoryDeadLetterStore{
		events: make(map[string]*DeadLetterEvent),
	}
}

func (s *MemoryDeadLetterStore) Store(ctx context.Context, dlq *DeadLetterEvent) (*DeadLetterEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if dlq.ID == "" {
		dlq.ID = uuid.NewString()
	}
	if dlq.CreatedAt.IsZero() {
		dlq.CreatedAt = time.Now().UTC()
	}

	clone := *dlq
	s.events[dlq.ID] = &clone
	return &clone, nil
}

func (s *MemoryDeadLetterStore) GetByID(ctx context.Context, id string) (*DeadLetterEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ev, ok := s.events[id]
	if !ok {
		return nil, ErrDLQNotFound
	}
	clone := *ev
	return &clone, nil
}

func (s *MemoryDeadLetterStore) List(ctx context.Context, filter DLQFilter) ([]*DeadLetterEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*DeadLetterEvent
	for _, ev := range s.events {
		if filter.TenantID != "" && ev.TenantID != "" && ev.TenantID != filter.TenantID {
			continue
		}
		if filter.Topic != "" && ev.Topic != filter.Topic {
			continue
		}
		if filter.Resolved != nil && ev.Resolved != *filter.Resolved {
			continue
		}
		clone := *ev
		result = append(result, &clone)
	}

	// Sort newest first
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	if filter.Offset > 0 {
		if filter.Offset >= len(result) {
			return []*DeadLetterEvent{}, nil
		}
		result = result[filter.Offset:]
	}

	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result, nil
}

func (s *MemoryDeadLetterStore) MarkResolved(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ev, ok := s.events[id]
	if !ok {
		return ErrDLQNotFound
	}
	ev.Resolved = true
	return nil
}

func (s *MemoryDeadLetterStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.events[id]; !ok {
		return ErrDLQNotFound
	}
	delete(s.events, id)
	return nil
}
