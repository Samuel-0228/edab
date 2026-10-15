package destinations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"automation-engine/internal/core/event"
)

// MockConfig allows configuring MockDestinationAdapter behavior via config JSON or programmatically.
type MockConfig struct {
	ShouldFail       bool   `json:"should_fail"`
	FailAttempts     int    `json:"fail_attempts"`
	FailErrorMessage string `json:"fail_error_message"`
}

// MockDestinationAdapter is a flexible destination adapter for testing and simulation.
type MockDestinationAdapter struct {
	mu              sync.Mutex
	connected       bool
	config          MockConfig
	deliveryCount   int
	deliveredEvents []*event.Event
}

func NewMockDestinationAdapter() *MockDestinationAdapter {
	return &MockDestinationAdapter{
		deliveredEvents: make([]*event.Event, 0),
	}
}

func (m *MockDestinationAdapter) Name() string {
	return "mock"
}

func (m *MockDestinationAdapter) Connect(ctx context.Context, config json.RawMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	if len(config) > 0 && string(config) != "{}" {
		_ = json.Unmarshal(config, &m.config)
	}
	return nil
}

func (m *MockDestinationAdapter) Health(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return fmt.Errorf("mock destination adapter is not connected")
	}
	return nil
}

// SetFailBehavior configures mock failure behavior programmatically for tests.
func (m *MockDestinationAdapter) SetFailBehavior(shouldFail bool, failAttempts int, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.ShouldFail = shouldFail
	m.config.FailAttempts = failAttempts
	m.config.FailErrorMessage = errMsg
}

func (m *MockDestinationAdapter) Deliver(ctx context.Context, ev *event.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.deliveryCount++

	if m.config.ShouldFail {
		msg := m.config.FailErrorMessage
		if msg == "" {
			msg = "simulated destination delivery failure"
		}
		return errors.New(msg)
	}

	if m.config.FailAttempts > 0 && m.deliveryCount <= m.config.FailAttempts {
		msg := m.config.FailErrorMessage
		if msg == "" {
			msg = fmt.Sprintf("simulated transient failure (attempt %d of %d)", m.deliveryCount, m.config.FailAttempts)
		}
		return errors.New(msg)
	}

	m.deliveredEvents = append(m.deliveredEvents, ev)
	return nil
}

// GetDeliveredEvents returns all events delivered to this mock destination.
func (m *MockDestinationAdapter) GetDeliveredEvents() []*event.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	cpy := make([]*event.Event, len(m.deliveredEvents))
	copy(cpy, m.deliveredEvents)
	return cpy
}

// GetDeliveryCount returns how many delivery attempts were made.
func (m *MockDestinationAdapter) GetDeliveryCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deliveryCount
}

// Reset clears delivery records and counts.
func (m *MockDestinationAdapter) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deliveryCount = 0
	m.deliveredEvents = make([]*event.Event, 0)
}
