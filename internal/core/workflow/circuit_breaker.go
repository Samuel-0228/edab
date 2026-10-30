package workflow

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open for integration")

// CircuitState represents the operational state of a circuit breaker.
type CircuitState string

const (
	StateClosed   CircuitState = "closed"
	StateOpen     CircuitState = "open"
	StateHalfOpen CircuitState = "half_open"
)

// CircuitBreakerConfig configures failure thresholds and reset timeouts.
type CircuitBreakerConfig struct {
	FailureThreshold int
	OpenTimeout      time.Duration
}

// CircuitBreaker guards an integration from cascaded failures.
type CircuitBreaker struct {
	mu               sync.RWMutex
	name             string
	state            CircuitState
	failureThreshold int
	consecutiveFails int
	openTimeout      time.Duration
	lastStateChange  time.Time
}

// NewCircuitBreaker creates a circuit breaker for an integration.
func NewCircuitBreaker(name string, cfg CircuitBreakerConfig) *CircuitBreaker {
	thresh := cfg.FailureThreshold
	if thresh <= 0 {
		thresh = 5
	}
	timeout := cfg.OpenTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &CircuitBreaker{
		name:             name,
		state:            StateClosed,
		failureThreshold: thresh,
		openTimeout:      timeout,
		lastStateChange:  time.Now(),
	}
}

// Allow checks if a request is permitted through the circuit breaker.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	switch cb.state {
	case StateClosed:
		return nil

	case StateOpen:
		if now.Sub(cb.lastStateChange) >= cb.openTimeout {
			cb.state = StateHalfOpen
			cb.lastStateChange = now
			return nil // Probe allowed
		}
		return fmt.Errorf("%w: %s (tripped due to %d consecutive failures)", ErrCircuitOpen, cb.name, cb.consecutiveFails)

	case StateHalfOpen:
		// Only one probe at a time
		return nil

	default:
		return nil
	}
}

// RecordSuccess records a successful call, resetting the circuit breaker to closed.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails = 0
	if cb.state != StateClosed {
		cb.state = StateClosed
		cb.lastStateChange = time.Now()
	}
}

// RecordFailure records a failed call. Trips the circuit breaker to open if threshold reached.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails++
	if cb.state == StateHalfOpen || cb.consecutiveFails >= cb.failureThreshold {
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
	}
}

// State returns current state and consecutive failure count.
func (cb *CircuitBreaker) State() (CircuitState, int) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state, cb.consecutiveFails
}

// CircuitBreakerRegistry manages circuit breakers per integration.
type CircuitBreakerRegistry struct {
	mu       sync.RWMutex
	cfg      CircuitBreakerConfig
	breakers map[string]*CircuitBreaker
}

// NewCircuitBreakerRegistry creates a registry with default config.
func NewCircuitBreakerRegistry(cfg CircuitBreakerConfig) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		cfg:      cfg,
		breakers: make(map[string]*CircuitBreaker),
	}
}

// Get returns or creates the circuit breaker for an integration.
func (r *CircuitBreakerRegistry) Get(integration string) *CircuitBreaker {
	r.mu.Lock()
	defer r.mu.Unlock()

	cb, exists := r.breakers[integration]
	if !exists {
		cb = NewCircuitBreaker(integration, r.cfg)
		r.breakers[integration] = cb
	}
	return cb
}

// Allow checks if the integration is permitted to execute.
func (r *CircuitBreakerRegistry) Allow(integration string) error {
	return r.Get(integration).Allow()
}

// RecordSuccess records a success for the integration.
func (r *CircuitBreakerRegistry) RecordSuccess(integration string) {
	r.Get(integration).RecordSuccess()
}

// RecordFailure records a failure for the integration.
func (r *CircuitBreakerRegistry) RecordFailure(integration string) {
	r.Get(integration).RecordFailure()
}

// Status returns a map of all circuit breakers and their state string.
func (r *CircuitBreakerRegistry) Status() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make(map[string]string, len(r.breakers))
	for name, cb := range r.breakers {
		state, _ := cb.State()
		res[name] = string(state)
	}
	return res
}
