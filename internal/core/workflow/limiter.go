package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrRateLimited           = errors.New("workflow rate limit exceeded")
	ErrMaxConcurrencyReached = errors.New("workflow max concurrency reached")
	ErrConcurrencyKeyBusy    = errors.New("concurrency key currently executing")
)

type tokenBucket struct {
	mu         sync.Mutex
	capacity   float64
	tokens     float64
	ratePerSec float64
	lastRefill time.Time
}

func newTokenBucket(capacity float64, ratePerSec float64) *tokenBucket {
	return &tokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		ratePerSec: ratePerSec,
		lastRefill: time.Now(),
	}
}

func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	tb.tokens = tb.tokens + elapsed*tb.ratePerSec
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

// TrafficController manages rate limits and concurrency limits for workflow executions.
type TrafficController struct {
	mu           sync.Mutex
	rateLimiters map[string]*tokenBucket
	activeWork   map[string]int
	activeKeys   map[string]int
}

// NewTrafficController creates a new traffic controller.
func NewTrafficController() *TrafficController {
	return &TrafficController{
		rateLimiters: make(map[string]*tokenBucket),
		activeWork:   make(map[string]int),
		activeKeys:   make(map[string]int),
	}
}

// TryAcquire attempts to acquire rate limit and concurrency permits for a workflow execution.
// If successful, returns a release function that must be called when execution finishes.
func (tc *TrafficController) TryAcquire(ctx context.Context, wf *Workflow, concurrencyKey string) (func(), error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// 1. Rate Limit check
	if wf.RateLimit > 0 {
		limiter, exists := tc.rateLimiters[wf.ID]
		if !exists {
			// wf.RateLimit is max executions per minute
			ratePerSec := float64(wf.RateLimit) / 60.0
			limiter = newTokenBucket(float64(wf.RateLimit), ratePerSec)
			tc.rateLimiters[wf.ID] = limiter
		}
		if !limiter.allow() {
			return nil, ErrRateLimited
		}
	}

	// 2. Max Concurrency check
	if wf.MaxConcurrency > 0 {
		current := tc.activeWork[wf.ID]
		if current >= wf.MaxConcurrency {
			return nil, ErrMaxConcurrencyReached
		}
	}

	// 3. Concurrency Key serialization check
	keyID := ""
	if concurrencyKey != "" {
		keyID = fmt.Sprintf("%s:%s", wf.ID, concurrencyKey)
		if tc.activeKeys[keyID] > 0 {
			return nil, ErrConcurrencyKeyBusy
		}
	}

	// Mark active
	tc.activeWork[wf.ID]++
	if keyID != "" {
		tc.activeKeys[keyID]++
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			tc.mu.Lock()
			defer tc.mu.Unlock()
			tc.activeWork[wf.ID]--
			if tc.activeWork[wf.ID] <= 0 {
				delete(tc.activeWork, wf.ID)
			}
			if keyID != "" {
				tc.activeKeys[keyID]--
				if tc.activeKeys[keyID] <= 0 {
					delete(tc.activeKeys, keyID)
				}
			}
		})
	}

	return release, nil
}

// ActiveCount returns current active executions for a workflow.
func (tc *TrafficController) ActiveCount(workflowID string) int {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.activeWork[workflowID]
}
