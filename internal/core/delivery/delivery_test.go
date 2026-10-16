package delivery

import (
	"testing"
	"time"
)

func TestNewJobDefaults(t *testing.T) {
	// Job with custom max attempts
	j1 := NewJob("ev-123", "dest-456", 5)
	if j1.ID == "" {
		t.Error("expected job ID to be generated")
	}
	if j1.Status != StatusPending {
		t.Errorf("expected StatusPending, got %s", j1.Status)
	}
	if j1.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", j1.Attempts)
	}
	if j1.MaxAttempts != 5 {
		t.Errorf("expected 5 max attempts, got %d", j1.MaxAttempts)
	}
	if j1.CreatedAt.IsZero() {
		t.Error("expected CreatedAt timestamp to be set")
	}

	// Job with invalid max attempts defaults to 3
	j2 := NewJob("ev-123", "dest-456", 0)
	if j2.MaxAttempts != 3 {
		t.Errorf("expected default max attempts 3, got %d", j2.MaxAttempts)
	}
}

func TestWorkerCalculateBackoff(t *testing.T) {
	cfg := WorkerConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
	}
	worker := NewWorker(nil, nil, nil, nil, cfg)

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{attempt: 0, expected: 100 * time.Millisecond},
		{attempt: 1, expected: 100 * time.Millisecond},
		{attempt: 2, expected: 200 * time.Millisecond},
		{attempt: 3, expected: 400 * time.Millisecond},
		{attempt: 4, expected: 800 * time.Millisecond},
		{attempt: 5, expected: 1000 * time.Millisecond},  // Capped at MaxBackoff
		{attempt: 10, expected: 1000 * time.Millisecond}, // Capped at MaxBackoff
	}

	for _, tt := range tests {
		got := worker.CalculateBackoff(tt.attempt)
		if got != tt.expected {
			t.Errorf("CalculateBackoff(%d) = %v, expected %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestWorkerDefaults(t *testing.T) {
	emptyCfg := WorkerConfig{}
	worker := NewWorker(nil, nil, nil, nil, emptyCfg)

	if worker.cfg.PollInterval != 1*time.Second {
		t.Errorf("expected default PollInterval 1s, got %v", worker.cfg.PollInterval)
	}
	if worker.cfg.Concurrency != 2 {
		t.Errorf("expected default Concurrency 2, got %d", worker.cfg.Concurrency)
	}
	if worker.cfg.InitialBackoff != 2*time.Second {
		t.Errorf("expected default InitialBackoff 2s, got %v", worker.cfg.InitialBackoff)
	}
	if worker.cfg.MaxBackoff != 1*time.Minute {
		t.Errorf("expected default MaxBackoff 1m, got %v", worker.cfg.MaxBackoff)
	}
}
