package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"kerob/internal/core/event"
)

// ScheduleConfig defines the timing configuration for a scheduled automation.
type ScheduleConfig struct {
	IntervalStr string `json:"interval,omitempty"` // e.g. "1m", "5m", "1h", "24h"
	CronExpr    string `json:"cron,omitempty"`     // cron expression e.g. "*/5 * * * *"
}

// Scheduler periodically evaluates workflows configured with trigger.type == "schedule"
// and emits normalized events to activate them.
type Scheduler struct {
	store    Store
	engine   *Engine
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	lastRun  map[string]time.Time
}

// NewScheduler creates an automated cron/interval scheduler.
func NewScheduler(store Store, engine *Engine, pollInterval time.Duration) *Scheduler {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	return &Scheduler{
		store:    store,
		engine:   engine,
		interval: pollInterval,
		stopCh:   make(chan struct{}),
		lastRun:  make(map[string]time.Time),
	}
}

// Start begins the scheduling evaluation loop.
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case now := <-ticker.C:
				s.tick(ctx, now)
			}
		}
	}()
	slog.Info("workflow scheduler started", slog.Duration("interval", s.interval))
}

// Stop terminates the scheduler background loop cleanly.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	slog.Info("workflow scheduler stopped")
}

func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	if s.store == nil || s.engine == nil {
		return
	}

	workflows, err := s.store.ListActive(ctx)
	if err != nil {
		slog.Error("scheduler failed to list active workflows", slog.String("error", err.Error()))
		return
	}

	for _, wf := range workflows {
		if wf.Trigger.Type != "schedule" {
			continue
		}

		if s.shouldRun(wf, now) {
			s.mu.Lock()
			s.lastRun[wf.ID] = now
			s.mu.Unlock()

			go func(w *Workflow, t time.Time) {
				payload := map[string]any{
					"scheduled_at": t.Format(time.RFC3339),
					"workflow_id":  w.ID,
					"workflow_name": w.Name,
				}
				extID := fmt.Sprintf("sched:%s:%d", w.ID, t.Unix())
				ev, err := event.New("scheduler", extID, "schedule.tick", payload, nil)
				if err != nil {
					slog.Error("failed to construct scheduled event", slog.String("error", err.Error()))
					return
				}

				if _, err := s.engine.ProcessEvent(ctx, ev); err != nil {
					slog.Error("scheduled workflow execution failed",
						slog.String("workflow_id", w.ID),
						slog.String("error", err.Error()),
					)
				}
			}(wf, now)
		}
	}
}

func (s *Scheduler) shouldRun(wf *Workflow, now time.Time) bool {
	s.mu.Lock()
	last, exists := s.lastRun[wf.ID]
	s.mu.Unlock()

	var cfg ScheduleConfig
	if len(wf.Trigger.Config) > 0 {
		_ = json.Unmarshal(wf.Trigger.Config, &cfg)
	}

	// 1. Simple interval evaluation
	if cfg.IntervalStr != "" {
		interval, err := time.ParseDuration(cfg.IntervalStr)
		if err == nil && interval > 0 {
			if !exists {
				return true
			}
			return now.Sub(last) >= interval
		}
	}

	// 2. Cron expression evaluation (standard 5-part cron: minute hour dom month dow)
	if cfg.CronExpr != "" {
		if matchesCron(cfg.CronExpr, now) {
			if !exists {
				return true
			}
			// Prevent multiple firings in the same minute
			return now.Sub(last) >= 59*time.Second
		}
		return false
	}

	// Default fallback: every 5 minutes if unconfigured
	defaultInterval := 5 * time.Minute
	if !exists {
		return true
	}
	return now.Sub(last) >= defaultInterval
}

// matchesCron provides standard cron field matching for min, hour, dom, month, dow
func matchesCron(expr string, t time.Time) bool {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return false
	}

	minMatch := matchCronField(fields[0], t.Minute(), 0, 59)
	hourMatch := matchCronField(fields[1], t.Hour(), 0, 23)
	domMatch := matchCronField(fields[2], t.Day(), 1, 31)
	monthMatch := matchCronField(fields[3], int(t.Month()), 1, 12)
	dowMatch := matchCronField(fields[4], int(t.Weekday()), 0, 6)

	return minMatch && hourMatch && domMatch && monthMatch && dowMatch
}

func matchCronField(pattern string, value int, min, max int) bool {
	if pattern == "*" {
		return true
	}

	// Handle step values like */5
	if strings.HasPrefix(pattern, "*/") {
		stepStr := strings.TrimPrefix(pattern, "*/")
		step, err := strconv.Atoi(stepStr)
		if err == nil && step > 0 {
			return (value % step) == 0
		}
		return false
	}

	// Handle comma-separated list like 1,2,5
	parts := strings.Split(pattern, ",")
	for _, part := range parts {
		if val, err := strconv.Atoi(part); err == nil && val == value {
			return true
		}
	}

	return false
}
