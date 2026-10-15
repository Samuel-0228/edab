package delivery

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrJobNotFound = errors.New("delivery job not found")
)

// Status represents the state of a delivery job.
type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusDelivered  Status = "delivered"
	StatusFailed     Status = "failed"
	StatusDeadLetter Status = "dead_letter"
)

// DeliveryJob represents an individual delivery task targeting one destination for an event.
type DeliveryJob struct {
	ID            string    `json:"id"`
	EventID       string    `json:"event_id"`
	DestinationID string    `json:"destination_id"`
	Status        Status    `json:"status"`
	Attempts      int       `json:"attempts"`
	MaxAttempts   int       `json:"max_attempts"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	LastError     *string   `json:"last_error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NewJob creates a new pending DeliveryJob.
func NewJob(eventID, destinationID string, maxAttempts int) *DeliveryJob {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	now := time.Now().UTC()
	return &DeliveryJob{
		ID:            uuid.NewString(),
		EventID:       eventID,
		DestinationID: destinationID,
		Status:        StatusPending,
		Attempts:      0,
		MaxAttempts:   maxAttempts,
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// ListFilter specifies filtering criteria for querying delivery jobs.
type ListFilter struct {
	EventID       string
	DestinationID string
	Status        Status
	Limit         int
	Offset        int
}

// Store defines database operations for delivery jobs.
type Store interface {
	CreateJob(ctx context.Context, job *DeliveryJob) error
	CreateJobs(ctx context.Context, jobs []*DeliveryJob) error
	GetByID(ctx context.Context, id string) (*DeliveryJob, error)
	List(ctx context.Context, filter ListFilter) ([]*DeliveryJob, error)
	ClaimNextJob(ctx context.Context) (*DeliveryJob, error)
	MarkDelivered(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, errStr string, nextAttempt time.Time) error
	MarkDeadLetter(ctx context.Context, id string, errStr string) error
	ResetForRetry(ctx context.Context, id string) (*DeliveryJob, error)
}
