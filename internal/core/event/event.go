package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrEventNotFound = errors.New("event not found")
)

// Event is the generic representation of an external event received by the engine.
// The core engine never depends on service-specific fields.
type Event struct {
	ID              string          `json:"id"`
	SourceID        string          `json:"source_id"`
	ExternalEventID string          `json:"external_event_id"`
	EventType       string          `json:"event_type"`
	Payload         json.RawMessage `json:"payload"`
	Metadata        json.RawMessage `json:"metadata"`
	CreatedAt       time.Time       `json:"created_at"`
}

// New creates a new Event instance with generated ID and timestamp.
func New(sourceID, externalEventID, eventType string, payload any, metadata any) (*Event, error) {
	if sourceID == "" {
		return nil, fmt.Errorf("source_id is required")
	}
	if externalEventID == "" {
		return nil, fmt.Errorf("external_event_id is required")
	}
	if eventType == "" {
		eventType = "default"
	}

	payloadBytes, err := marshalJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	metaBytes, err := marshalJSON(metadata)
	if err != nil {
		return nil, fmt.Errorf("invalid metadata: %w", err)
	}

	return &Event{
		ID:              uuid.NewString(),
		SourceID:        sourceID,
		ExternalEventID: externalEventID,
		EventType:       eventType,
		Payload:         payloadBytes,
		Metadata:        metaBytes,
		CreatedAt:       time.Now().UTC(),
	}, nil
}

func marshalJSON(v any) (json.RawMessage, error) {
	if v == nil {
		return json.RawMessage("{}"), nil
	}
	switch val := v.(type) {
	case json.RawMessage:
		if len(val) == 0 {
			return json.RawMessage("{}"), nil
		}
		return val, nil
	case []byte:
		if len(val) == 0 {
			return json.RawMessage("{}"), nil
		}
		if !json.Valid(val) {
			// If not valid json, wrap as json string
			b, err := json.Marshal(string(val))
			return json.RawMessage(b), err
		}
		return json.RawMessage(val), nil
	case string:
		if json.Valid([]byte(val)) {
			return json.RawMessage(val), nil
		}
		b, err := json.Marshal(val)
		return json.RawMessage(b), err
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(b), nil
	}
}

// ListFilter specifies filtering criteria for querying events.
type ListFilter struct {
	SourceID  string
	EventType string
	Limit     int
	Offset    int
}

// Store defines persistence operations for Events.
type Store interface {
	// Create persists an event. Returns (persistedEvent, inserted, error).
	// If (source_id, external_event_id) already exists, inserted is false,
	// returning the existing event without error (idempotency).
	Create(ctx context.Context, e *Event) (*Event, bool, error)
	GetByID(ctx context.Context, id string) (*Event, error)
	GetByExternalID(ctx context.Context, sourceID, externalID string) (*Event, error)
	List(ctx context.Context, filter ListFilter) ([]*Event, error)
}
