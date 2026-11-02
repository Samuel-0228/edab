package streaming

import (
	"context"
	"time"
)

// MessageHandler is called for each consumed stream record.
// If it returns an error, the consumer initiates retry or routes to DLQ.
type MessageHandler func(ctx context.Context, record *Record) error

// Producer defines outbound message publishing onto partitioned event streams.
type Producer interface {
	Publish(ctx context.Context, record *Record) (*PublishResult, error)
	PublishBatch(ctx context.Context, records []*Record) ([]*PublishResult, error)
	Close() error
}

// Consumer handles subscribing to topics and partitioned stream consumption with offset commits.
type Consumer interface {
	Subscribe(ctx context.Context, groupID string, topics []string, handler MessageHandler) error
	CommitOffset(ctx context.Context, groupID, topic string, partition int32, offset int64) error
	GetCommittedOffset(ctx context.Context, groupID, topic string, partition int32) (int64, error)
	Seek(ctx context.Context, topic string, partition int32, offset int64) error
	Close() error
}

// StreamAdapter encapsulates a streaming engine backend (memory, WAL, Kafka, NATS).
type StreamAdapter interface {
	Name() string
	Producer() (Producer, error)
	Consumer() (Consumer, error)
	Close() error
}

// OffsetStore manages durable persistence of consumer group offsets across restarts.
type OffsetStore interface {
	CommitOffset(ctx context.Context, offset ConsumerOffset) error
	GetOffset(ctx context.Context, groupID, topic string, partition int32) (int64, error)
	ListOffsets(ctx context.Context, groupID string) ([]ConsumerOffset, error)
}

// DLQFilter specifies query criteria for listing dead-letter stream records.
type DLQFilter struct {
	TenantID string
	Topic    string
	Resolved *bool
	Limit    int
	Offset   int
}

// DeadLetterStore persists unprocessable streaming records with execution failure context.
type DeadLetterStore interface {
	Store(ctx context.Context, dlq *DeadLetterEvent) (*DeadLetterEvent, error)
	GetByID(ctx context.Context, id string) (*DeadLetterEvent, error)
	List(ctx context.Context, filter DLQFilter) ([]*DeadLetterEvent, error)
	MarkResolved(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

// ReplayFilter specifies event stream replay parameters.
type ReplayFilter struct {
	TenantID          string    `json:"tenant_id,omitempty"`
	Topic             string    `json:"topic,omitempty"`
	SourceID          string    `json:"source_id,omitempty"`
	EventType         string    `json:"event_type,omitempty"`
	FromTime          time.Time `json:"from_time,omitempty"`
	ToTime            time.Time `json:"to_time,omitempty"`
	Partition         *int32    `json:"partition,omitempty"`
	FromOffset        *int64    `json:"from_offset,omitempty"`
	ToOffset          *int64    `json:"to_offset,omitempty"`
	BypassIdempotency bool      `json:"bypass_idempotency"`
	Limit             int       `json:"limit,omitempty"`
}
