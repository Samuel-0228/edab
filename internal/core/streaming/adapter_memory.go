package streaming

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrClosed = errors.New("stream adapter closed")
)

// MemoryStreamAdapter implements an in-memory partitioned stream broker.
// It maintains per-partition message queues, consumer group offset commits, and deterministic ordering.
type MemoryStreamAdapter struct {
	mu           sync.RWMutex
	partitions   int
	topics       map[string]map[int32][]*Record
	groupOffsets map[string]map[string]map[int32]int64 // [group][topic][partition] -> offset
	listeners    map[string]map[int32][]chan struct{}  // [topic][partition] -> notify channels
	partitioner  PartitionStrategy
	closed       bool
	cancel       context.CancelFunc
	ctx          context.Context
}

// NewMemoryStreamAdapter creates a new in-memory streaming broker.
func NewMemoryStreamAdapter(numPartitions int) *MemoryStreamAdapter {
	if numPartitions <= 0 {
		numPartitions = 4
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &MemoryStreamAdapter{
		partitions:   numPartitions,
		topics:       make(map[string]map[int32][]*Record),
		groupOffsets: make(map[string]map[string]map[int32]int64),
		listeners:    make(map[string]map[int32][]chan struct{}),
		partitioner:  DefaultPartitionStrategy,
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (m *MemoryStreamAdapter) Name() string {
	return "memory"
}

func (m *MemoryStreamAdapter) Producer() (Producer, error) {
	return m, nil
}

func (m *MemoryStreamAdapter) Consumer() (Consumer, error) {
	return m, nil
}

func (m *MemoryStreamAdapter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	m.cancel()
	return nil
}

// Publish writes a single record into the target partition.
func (m *MemoryStreamAdapter) Publish(ctx context.Context, record *Record) (*PublishResult, error) {
	results, err := m.PublishBatch(ctx, []*Record{record})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// PublishBatch writes multiple records atomically per partition.
func (m *MemoryStreamAdapter) PublishBatch(ctx context.Context, records []*Record) ([]*PublishResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrClosed
	}

	results := make([]*PublishResult, 0, len(records))
	now := time.Now().UTC()

	// Track partitions that received new records to notify listeners
	notified := make(map[string]map[int32]bool)

	for _, rec := range records {
		if rec.Topic == "" {
			rec.Topic = "events"
		}
		if rec.ID == "" {
			rec.ID = uuid.NewString()
		}
		if rec.Timestamp.IsZero() {
			rec.Timestamp = now
		}

		// Assign partition if not explicitly provided
		if rec.Partition < 0 || rec.Partition >= int32(m.partitions) {
			rec.Partition = m.partitioner.AssignPartition(rec.OrderingKey, m.partitions)
		}

		// Ensure topic and partition exist
		if _, ok := m.topics[rec.Topic]; !ok {
			m.topics[rec.Topic] = make(map[int32][]*Record)
		}
		partRecords := m.topics[rec.Topic][rec.Partition]

		rec.Offset = int64(len(partRecords))
		m.topics[rec.Topic][rec.Partition] = append(partRecords, rec)

		results = append(results, &PublishResult{
			ID:        rec.ID,
			Topic:     rec.Topic,
			Partition: rec.Partition,
			Offset:    rec.Offset,
			Timestamp: rec.Timestamp,
		})

		if _, ok := notified[rec.Topic]; !ok {
			notified[rec.Topic] = make(map[int32]bool)
		}
		notified[rec.Topic][rec.Partition] = true
	}

	// Wake up waiting partition listeners
	for topic, parts := range notified {
		for part := range parts {
			if topicListeners, ok := m.listeners[topic]; ok {
				for _, ch := range topicListeners[part] {
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			}
		}
	}

	return results, nil
}

// Subscribe listens to specified topics using a consumer group, processing records concurrently per partition.
func (m *MemoryStreamAdapter) Subscribe(ctx context.Context, groupID string, topics []string, handler MessageHandler) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrClosed
	}

	// Initialize group offsets map
	if _, ok := m.groupOffsets[groupID]; !ok {
		m.groupOffsets[groupID] = make(map[string]map[int32]int64)
	}

	// For each topic and each partition, spawn a dedicated consumer goroutine
	for _, topic := range topics {
		if _, ok := m.groupOffsets[groupID][topic]; !ok {
			m.groupOffsets[groupID][topic] = make(map[int32]int64)
		}
		if _, ok := m.listeners[topic]; !ok {
			m.listeners[topic] = make(map[int32][]chan struct{})
		}

		for p := int32(0); p < int32(m.partitions); p++ {
			part := p
			top := topic
			ch := make(chan struct{}, 10)
			m.listeners[top][part] = append(m.listeners[top][part], ch)

			go m.consumePartition(ctx, groupID, top, part, ch, handler)
		}
	}
	m.mu.Unlock()

	return nil
}

func (m *MemoryStreamAdapter) consumePartition(ctx context.Context, groupID, topic string, partition int32, notify chan struct{}, handler MessageHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.ctx.Done():
			return
		default:
		}

		// Read next records starting from committed offset
		record := m.readNextRecord(groupID, topic, partition)
		if record == nil {
			// Wait for new messages or context cancellation
			select {
			case <-notify:
				continue
			case <-time.After(200 * time.Millisecond):
				continue
			case <-ctx.Done():
				return
			case <-m.ctx.Done():
				return
			}
		}

		// Process record through handler
		if err := handler(ctx, record); err == nil {
			// Auto-commit offset upon success
			_ = m.CommitOffset(ctx, groupID, topic, partition, record.Offset+1)
		}
	}
}

func (m *MemoryStreamAdapter) readNextRecord(groupID, topic string, partition int32) *Record {
	m.mu.RLock()
	defer m.mu.RUnlock()

	offset := int64(0)
	if groupMap, ok := m.groupOffsets[groupID]; ok {
		if topMap, ok := groupMap[topic]; ok {
			offset = topMap[partition]
		}
	}

	if topParts, ok := m.topics[topic]; ok {
		if records, ok := topParts[partition]; ok {
			if offset < int64(len(records)) {
				return records[offset]
			}
		}
	}
	return nil
}

func (m *MemoryStreamAdapter) CommitOffset(ctx context.Context, groupID, topic string, partition int32, offset int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.groupOffsets[groupID]; !ok {
		m.groupOffsets[groupID] = make(map[string]map[int32]int64)
	}
	if _, ok := m.groupOffsets[groupID][topic]; !ok {
		m.groupOffsets[groupID][topic] = make(map[int32]int64)
	}

	m.groupOffsets[groupID][topic][partition] = offset
	return nil
}

func (m *MemoryStreamAdapter) GetCommittedOffset(ctx context.Context, groupID, topic string, partition int32) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if groupMap, ok := m.groupOffsets[groupID]; ok {
		if topMap, ok := groupMap[topic]; ok {
			return topMap[partition], nil
		}
	}
	return 0, nil
}

func (m *MemoryStreamAdapter) Seek(ctx context.Context, topic string, partition int32, offset int64) error {
	// Updates offset across all groups or for default consumption
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, topMap := range m.groupOffsets {
		if pMap, ok := topMap[topic]; ok {
			pMap[partition] = offset
		}
	}
	return nil
}

// GetRecords retrieves a range of records from a partition (used for replay and inspection).
func (m *MemoryStreamAdapter) GetRecords(topic string, partition int32, fromOffset, toOffset int64) []*Record {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Record
	if topParts, ok := m.topics[topic]; ok {
		if records, ok := topParts[partition]; ok {
			for _, rec := range records {
				if rec.Offset >= fromOffset && (toOffset <= 0 || rec.Offset <= toOffset) {
					result = append(result, rec)
				}
			}
		}
	}
	return result
}
