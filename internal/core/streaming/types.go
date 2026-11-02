package streaming

import (
	"encoding/json"
	"hash/fnv"
	"time"

	"kerob/internal/core/event"
)

// Record represents an individual message published to or consumed from an event stream.
type Record struct {
	ID          string            `json:"id"`
	Topic       string            `json:"topic"`
	Partition   int32             `json:"partition"`
	OrderingKey string            `json:"ordering_key,omitempty"`
	Offset      int64             `json:"offset"`
	Event       *event.Event      `json:"event"`
	Headers     map[string]string `json:"headers,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
}

// PublishResult contains the broker metadata returned when a record is successfully published.
type PublishResult struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Partition int32     `json:"partition"`
	Offset    int64     `json:"offset"`
	Timestamp time.Time `json:"timestamp"`
}

// ConsumerOffset tracks the high-watermark offset processed by a consumer group for a partition.
type ConsumerOffset struct {
	GroupID     string    `json:"group_id"`
	Topic       string    `json:"topic"`
	Partition   int32     `json:"partition"`
	Offset      int64     `json:"offset"`
	CommittedAt time.Time `json:"committed_at"`
}

// DeadLetterEvent stores an event that failed streaming validation or execution.
type DeadLetterEvent struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id,omitempty"`
	Topic       string          `json:"topic"`
	Partition   int32           `json:"partition"`
	Offset      int64           `json:"offset"`
	EventID     string          `json:"event_id"`
	OrderingKey string          `json:"ordering_key,omitempty"`
	EventType   string          `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
	ErrorReason string          `json:"error_reason"`
	RetryCount  int             `json:"retry_count"`
	Resolved    bool            `json:"resolved"`
	CreatedAt   time.Time       `json:"created_at"`
}

// PartitionStrategy calculates deterministic partition assignment based on ordering key.
type PartitionStrategy interface {
	AssignPartition(orderingKey string, numPartitions int) int32
}

// FNVHashPartitionStrategy assigns partitions via FNV-1a 32-bit hash.
type FNVHashPartitionStrategy struct{}

func (s FNVHashPartitionStrategy) AssignPartition(orderingKey string, numPartitions int) int32 {
	if numPartitions <= 1 {
		return 0
	}
	if orderingKey == "" {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(orderingKey))
	val := h.Sum32()
	return int32(val % uint32(numPartitions))
}

// DefaultPartitionStrategy is the platform standard partitioner.
var DefaultPartitionStrategy PartitionStrategy = FNVHashPartitionStrategy{}
