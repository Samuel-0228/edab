package workflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"kerob/internal/core/credentials"
)

func TestSchedulerMatchesCron(t *testing.T) {
	// 2026-10-21 14:15:00 (Wednesday)
	now := time.Date(2026, 10, 21, 14, 15, 0, 0, time.UTC)

	tests := []struct {
		cron  string
		match bool
	}{
		{"* * * * *", true},
		{"*/5 * * * *", true},
		{"*/10 * * * *", false},
		{"15 14 * * *", true},
		{"16 14 * * *", false},
		{"15 * * * 3", true}, // Wednesday = 3
		{"15 * * * 4", false},
	}

	for _, tt := range tests {
		got := matchesCron(tt.cron, now)
		if got != tt.match {
			t.Errorf("matchesCron(%q, %v) = %v; want %v", tt.cron, now, got, tt.match)
		}
	}
}

func TestSchedulerExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	mockExecutor := &MockActionExecutor{TypeStr: "http"}
	registry.Register(mockExecutor)

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)

	// Create a scheduled workflow
	schedConfig, _ := json.Marshal(ScheduleConfig{IntervalStr: "10ms"})
	_, err := wfStore.Create(ctx, &Workflow{
		Name:    "Scheduled Heartbeat",
		Enabled: true,
		Trigger: TriggerConfig{
			Type:   "schedule",
			Config: schedConfig,
		},
		Actions: []ActionStep{
			{
				Integration: "http",
				ActionType:  "http_request",
				Config:      json.RawMessage(`{"url":"https://api.example.com"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create scheduled workflow: %v", err)
	}

	scheduler := NewScheduler(wfStore, engine, 15*time.Millisecond)
	scheduler.Start(ctx)

	// Allow scheduler to tick at least once
	time.Sleep(50 * time.Millisecond)
	scheduler.Stop()

	if mockExecutor.Executed == 0 {
		t.Errorf("expected scheduled workflow to execute at least once, got %d", mockExecutor.Executed)
	}
}
