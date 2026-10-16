package telegram

import (
	"context"
	"sync"
	"time"
)

// RateLimiter enforces a global request rate and a per-chat request rate so
// that a single event routed to many destinations does not flood the Telegram
// API all at once. It is safe for concurrent use.
type RateLimiter struct {
	mu              sync.Mutex
	globalInterval  time.Duration
	perChatInterval time.Duration
	lastGlobal      time.Time
	lastPerChat     map[int64]time.Time
}

// NewRateLimiter builds a limiter from messages-per-second budgets. A rate of
// zero or less disables that dimension.
func NewRateLimiter(globalPerSecond, perChatPerSecond float64) *RateLimiter {
	return &RateLimiter{
		globalInterval:  intervalFromRate(globalPerSecond),
		perChatInterval: intervalFromRate(perChatPerSecond),
		lastPerChat:     make(map[int64]time.Time),
	}
}

func intervalFromRate(perSecond float64) time.Duration {
	if perSecond <= 0 {
		return 0
	}
	return time.Duration(float64(time.Second) / perSecond)
}

// Wait blocks until both the global and per-chat budgets allow a new request,
// or until the context is cancelled.
func (r *RateLimiter) Wait(ctx context.Context, chatID int64) error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	now := time.Now()
	var wait time.Duration

	if r.globalInterval > 0 {
		if next := r.lastGlobal.Add(r.globalInterval); next.After(now) {
			wait = time.Until(next)
		}
	}
	if r.perChatInterval > 0 {
		if next := r.lastPerChat[chatID].Add(r.perChatInterval); next.After(now) {
			if d := time.Until(next); d > wait {
				wait = d
			}
		}
	}

	release := now.Add(wait)
	r.lastGlobal = release
	r.lastPerChat[chatID] = release
	r.mu.Unlock()

	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
