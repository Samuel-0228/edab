package tests

import (
	"context"
	"encoding/json"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/telegram"
)

// TestDeduplicationSameMessage verifies that receiving the exact same Telegram
// message twice creates only 1 event and does not enqueue duplicate jobs.
func TestDeduplicationSameMessage(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()

	srcStore, destStore, routeStore, evStore, jobStore, pipe, worker := setupTelegramEngineTest(mockAPI)

	srcA, _ := srcStore.Create(ctx, &sources.Source{
		Name:        "Dedupe Source",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100777"}`),
		Enabled:     true,
	})
	dest1, _ := destStore.Create(ctx, &destinations.Destination{
		Name:        "Dedupe Target",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100888"}`),
		Enabled:     true,
	})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: srcA.ID, DestinationID: dest1.ID, Enabled: true})

	msg := tgbotapi.Message{
		MessageID: 444,
		Text:      "Duplicate Test",
		Chat:      &tgbotapi.Chat{ID: -100777, Title: "Dedupe Source", Type: "channel"},
	}
	rawMsg, _ := json.Marshal(msg)
	rawPost, _ := json.Marshal(telegram.RawPost{
		ChatID:   -100777,
		Messages: []json.RawMessage{rawMsg},
	})

	// First ingestion
	ev1, err := pipe.IngestRaw(ctx, srcA.ID, rawPost)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}

	// Second ingestion of identical message
	ev2, err := pipe.IngestRaw(ctx, srcA.ID, rawPost)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}

	if ev1.ID != ev2.ID {
		t.Fatalf("expected identical event returned on duplicate, got ev1=%s ev2=%s", ev1.ID, ev2.ID)
	}

	// Verify only 1 event stored
	events, _ := evStore.List(ctx, event.ListFilter{SourceID: srcA.ID})
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 event stored, got %d", len(events))
	}

	// Verify only 1 delivery job exists
	jobs, _ := jobStore.List(ctx, delivery.ListFilter{EventID: ev1.ID})
	if len(jobs) != 1 {
		t.Fatalf("expected 1 delivery job, got %d", len(jobs))
	}

	// Worker processes job
	claimed, _ := worker.ProcessNextJob(ctx)
	if !claimed {
		t.Fatal("worker should claim job")
	}

	// No more jobs ready
	claimed2, _ := worker.ProcessNextJob(ctx)
	if claimed2 {
		t.Fatal("no second job should exist")
	}

	if len(mockAPI.CopiedMessages) != 1 {
		t.Fatalf("expected exactly 1 delivery call, got %d", len(mockAPI.CopiedMessages))
	}
}

// TestDeliveryIdempotencyOnRetry verifies that retrying an already completed job
// uses the recorded result and avoids creating duplicate posts at the destination.
func TestDeliveryIdempotencyOnRetry(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()

	srcStore, destStore, routeStore, _, jobStore, pipe, worker := setupTelegramEngineTest(mockAPI)

	src, _ := srcStore.Create(ctx, &sources.Source{
		Name:        "Retry Src",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100500"}`),
		Enabled:     true,
	})
	dest, _ := destStore.Create(ctx, &destinations.Destination{
		Name:        "Retry Dst",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100600"}`),
		Enabled:     true,
	})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: src.ID, DestinationID: dest.ID, Enabled: true})

	msg := tgbotapi.Message{
		MessageID: 1234,
		Text:      "Retry Idempotency Test",
		Chat:      &tgbotapi.Chat{ID: -100500, Title: "Retry Src", Type: "channel"},
	}
	rawMsg, _ := json.Marshal(msg)
	rawPost, _ := json.Marshal(telegram.RawPost{
		ChatID:   -100500,
		Messages: []json.RawMessage{rawMsg},
	})

	ev, _ := pipe.IngestRaw(ctx, src.ID, rawPost)

	// Deliver once
	claimed, err := worker.ProcessNextJob(ctx)
	if err != nil || !claimed {
		t.Fatalf("initial delivery failed: claimed=%v err=%v", claimed, err)
	}

	if len(mockAPI.CopiedMessages) != 1 {
		t.Fatalf("expected 1 delivery call, got %d", len(mockAPI.CopiedMessages))
	}

	// Inspect the job: Result must be recorded
	jobs, _ := jobStore.List(ctx, delivery.ListFilter{EventID: ev.ID})
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	job := jobs[0]
	if len(job.Result) == 0 {
		t.Fatal("expected delivery result to be recorded on successful job")
	}

	// Reset for retry (as done by POST /deliveries/{id}/retry)
	retriedJob, err := jobStore.ResetForRetry(ctx, job.ID)
	if err != nil {
		t.Fatalf("reset for retry failed: %v", err)
	}
	if retriedJob.Status != delivery.StatusPending {
		t.Fatalf("expected status pending, got %s", retriedJob.Status)
	}

	// Worker processes retried job
	claimed2, err2 := worker.ProcessNextJob(ctx)
	if err2 != nil || !claimed2 {
		t.Fatalf("worker failed to claim retried job: %v", err2)
	}

	// Crucial check: mockAPI must NOT have received a second delivery request!
	if len(mockAPI.CopiedMessages) != 1 {
		t.Fatalf("duplicate post detected! Expected CopiedMessages count=1, got %d", len(mockAPI.CopiedMessages))
	}
}
