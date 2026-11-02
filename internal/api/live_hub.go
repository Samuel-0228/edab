package api

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"kerob/internal/core/workflow"
	"kerob/internal/observability"
)

// OperationalEvent represents an event broadcast to real-time operations subscribers.
type OperationalEvent struct {
	Type        string                  `json:"type"`
	TenantID    string                  `json:"tenant_id,omitempty"`
	ExecutionID string                  `json:"execution_id,omitempty"`
	WorkflowID  string                  `json:"workflow_id,omitempty"`
	Step        *workflow.ExecutionStep `json:"step,omitempty"`
	Status      string                  `json:"status,omitempty"`
	Error       *string                 `json:"error,omitempty"`
	Payload     any                     `json:"payload,omitempty"`
	Timestamp   time.Time               `json:"timestamp"`
	SequenceID  uint64                  `json:"sequence_id"`
}

// LiveSubscriber manages an active real-time connection receiving operational events.
type LiveSubscriber struct {
	id       string
	tenantID string
	Events   chan *OperationalEvent
	closed   atomic.Bool
}

// C returns a receive-only channel of operational events.
func (s *LiveSubscriber) C() <-chan *OperationalEvent {
	return s.Events
}

// LiveHub coordinates thread-safe pub/sub broadcasting of real-time operational events
// with tenant isolation and non-blocking backpressure handling.
type LiveHub struct {
	mu          sync.RWMutex
	subscribers map[string]*LiveSubscriber
	sequence    atomic.Uint64
}

// NewLiveHub creates an initialized LiveHub.
func NewLiveHub() *LiveHub {
	return &LiveHub{
		subscribers: make(map[string]*LiveSubscriber),
	}
}

// Subscribe registers a new subscriber channel filtered to the given tenant.
// If tenantID is empty, it receives events for the default or unassigned tenant.
// The returned cleanup function unregisters the subscriber.
func (h *LiveHub) Subscribe(tenantID string) (*LiveSubscriber, func()) {
	subID := uuid.NewString()
	sub := &LiveSubscriber{
		id:       subID,
		tenantID: tenantID,
		Events:   make(chan *OperationalEvent, 128),
	}

	h.mu.Lock()
	h.subscribers[subID] = sub
	h.mu.Unlock()

	observability.RecordLiveSubscriberConnected()

	cleanup := func() {
		if sub.closed.CompareAndSwap(false, true) {
			h.mu.Lock()
			delete(h.subscribers, subID)
			h.mu.Unlock()

			close(sub.Events)
			observability.RecordLiveSubscriberDisconnected()
		}
	}

	return sub, cleanup
}

// SubscriberCount returns the current count of active real-time subscribers.
func (h *LiveHub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}

// Broadcast dispatches an operational event to all eligible tenant subscribers.
func (h *LiveHub) Broadcast(evt *OperationalEvent) {
	if h == nil || evt == nil {
		return
	}

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}
	evt.SequenceID = h.sequence.Add(1)

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, sub := range h.subscribers {
		if sub.closed.Load() {
			continue
		}

		// Enforce tenant isolation: if subscriber has a specific tenant and event has a specific tenant,
		// they must match.
		if sub.tenantID != "" && evt.TenantID != "" && sub.tenantID != evt.TenantID {
			continue
		}

		// Non-blocking write to prevent slow consumer stalls
		select {
		case sub.Events <- evt:
		default:
			// Buffer full: drop event for slow subscriber to avoid pipeline stalling
		}
	}
}

// --- ExecutionEventListener Implementation ---

func (h *LiveHub) OnExecutionStarted(exec *workflow.Execution) {
	if exec == nil {
		return
	}
	h.Broadcast(&OperationalEvent{
		Type:        "execution.started",
		TenantID:    exec.TenantID,
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		Status:      string(exec.Status),
	})
}

func (h *LiveHub) OnExecutionStep(exec *workflow.Execution, step *workflow.ExecutionStep) {
	tenantID := ""
	workflowID := ""
	if exec != nil {
		tenantID = exec.TenantID
		workflowID = exec.WorkflowID
	}
	h.Broadcast(&OperationalEvent{
		Type:        "execution.step",
		TenantID:    tenantID,
		ExecutionID: step.ExecutionID,
		WorkflowID:  workflowID,
		Step:        step,
		Status:      step.Status,
		Error:       step.Error,
	})
}

func (h *LiveHub) OnExecutionCompleted(exec *workflow.Execution) {
	if exec == nil {
		return
	}
	h.Broadcast(&OperationalEvent{
		Type:        "execution.completed",
		TenantID:    exec.TenantID,
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		Status:      string(exec.Status),
	})
}

func (h *LiveHub) OnExecutionFailed(exec *workflow.Execution, errStr string) {
	tenantID := ""
	execID := ""
	wfID := ""
	if exec != nil {
		tenantID = exec.TenantID
		execID = exec.ID
		wfID = exec.WorkflowID
	}
	h.Broadcast(&OperationalEvent{
		Type:        "execution.failed",
		TenantID:    tenantID,
		ExecutionID: execID,
		WorkflowID:  wfID,
		Status:      string(workflow.ExecutionFailed),
		Error:       &errStr,
	})
}

func (h *LiveHub) OnExecutionRetrying(exec *workflow.Execution, nextAttempt time.Time) {
	tenantID := ""
	execID := ""
	wfID := ""
	if exec != nil {
		tenantID = exec.TenantID
		execID = exec.ID
		wfID = exec.WorkflowID
	}
	h.Broadcast(&OperationalEvent{
		Type:        "execution.retrying",
		TenantID:    tenantID,
		ExecutionID: execID,
		WorkflowID:  wfID,
		Status:      string(workflow.ExecutionRetrying),
		Payload: map[string]any{
			"next_attempt_at": nextAttempt,
			"attempts":        exec.Attempts,
		},
	})
}

func (h *LiveHub) OnExecutionPaused(exec *workflow.Execution) {
	if exec == nil {
		return
	}
	h.Broadcast(&OperationalEvent{
		Type:        "execution.paused",
		TenantID:    exec.TenantID,
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		Status:      string(workflow.ExecutionPaused),
	})
}

func (h *LiveHub) OnExecutionResumed(exec *workflow.Execution) {
	if exec == nil {
		return
	}
	h.Broadcast(&OperationalEvent{
		Type:        "execution.resumed",
		TenantID:    exec.TenantID,
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		Status:      string(workflow.ExecutionPending),
	})
}

func (h *LiveHub) OnExecutionCancelled(exec *workflow.Execution) {
	if exec == nil {
		return
	}
	h.Broadcast(&OperationalEvent{
		Type:        "execution.cancelled",
		TenantID:    exec.TenantID,
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		Status:      string(workflow.ExecutionCancelled),
	})
}

// BroadcastDLQ emits real-time dead-letter events.
func (h *LiveHub) BroadcastDLQ(eventType string, tenantID string, dl *workflow.DeadLetterRecord) {
	if h == nil || dl == nil {
		return
	}
	h.Broadcast(&OperationalEvent{
		Type:        eventType,
		TenantID:    tenantID,
		ExecutionID: dl.ExecutionID,
		WorkflowID:  dl.WorkflowID,
		Error:       &dl.Error,
		Payload:     dl,
	})
}
