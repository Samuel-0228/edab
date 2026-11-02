package workflow

import (
	"context"
	"sync"
)

type activeState struct {
	cancel context.CancelFunc
	paused bool
}

// ActiveExecutionRegistry tracks locally executing workflows so cancellations and pauses
// can propagate immediately to running steps, HTTP requests, and wait timers.
type ActiveExecutionRegistry struct {
	mu      sync.RWMutex
	actives map[string]*activeState
}

// NewActiveExecutionRegistry initializes an active execution context registry.
func NewActiveExecutionRegistry() *ActiveExecutionRegistry {
	return &ActiveExecutionRegistry{
		actives: make(map[string]*activeState),
	}
}

// Register registers an active execution run and its cancellation function.
func (r *ActiveExecutionRegistry) Register(id string, cancel context.CancelFunc) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actives[id] = &activeState{
		cancel: cancel,
		paused: false,
	}
}

// Unregister removes the execution context once it has finished.
func (r *ActiveExecutionRegistry) Unregister(id string) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.actives, id)
}

// Cancel cancels the active execution context if it is running locally.
func (r *ActiveExecutionRegistry) Cancel(id string) bool {
	if r == nil || id == "" {
		return false
	}
	r.mu.RLock()
	state, exists := r.actives[id]
	r.mu.RUnlock()

	if exists && state != nil && state.cancel != nil {
		state.cancel()
		return true
	}
	return false
}

// Pause marks the execution as paused and cancels the active context to interrupt in-flight steps.
func (r *ActiveExecutionRegistry) Pause(id string) bool {
	if r == nil || id == "" {
		return false
	}
	r.mu.Lock()
	state, exists := r.actives[id]
	if exists && state != nil {
		state.paused = true
		if state.cancel != nil {
			state.cancel()
		}
	}
	r.mu.Unlock()
	return exists
}

// IsPaused reports whether the execution was flagged as paused.
func (r *ActiveExecutionRegistry) IsPaused(id string) bool {
	if r == nil || id == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if state, exists := r.actives[id]; exists && state != nil {
		return state.paused
	}
	return false
}

// IsActive reports whether the execution is currently registered on this node.
func (r *ActiveExecutionRegistry) IsActive(id string) bool {
	if r == nil || id == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.actives[id]
	return exists
}

// ActiveIDs returns a list of all active execution IDs running on this node.
func (r *ActiveExecutionRegistry) ActiveIDs() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.actives))
	for id := range r.actives {
		ids = append(ids, id)
	}
	return ids
}
