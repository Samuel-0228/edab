package destinations

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"automation-engine/internal/core/event"
)

// StdoutDestinationAdapter formats and writes delivered events to logs/stdout.
type StdoutDestinationAdapter struct {
	mu        sync.Mutex
	connected bool
}

func NewStdoutDestinationAdapter() *StdoutDestinationAdapter {
	return &StdoutDestinationAdapter{}
}

func (s *StdoutDestinationAdapter) Name() string {
	return "stdout"
}

func (s *StdoutDestinationAdapter) Connect(ctx context.Context, config json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = true
	return nil
}

func (s *StdoutDestinationAdapter) Health(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.connected {
		return fmt.Errorf("stdout destination adapter is not connected")
	}
	return nil
}

func (s *StdoutDestinationAdapter) Deliver(ctx context.Context, ev *event.Event) error {
	slog.Info("stdout destination delivered event",
		slog.String("event_id", ev.ID),
		slog.String("source_id", ev.SourceID),
		slog.String("event_type", ev.EventType),
		slog.String("payload", string(ev.Payload)),
	)
	return nil
}
