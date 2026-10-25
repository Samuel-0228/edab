package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
)

func TestEventsAndDeliveriesAPI(t *testing.T) {
	server, _, cfg := setupTestServer()
	handler := server.Handler()
	ctx := context.Background()

	// 1. Ingest Event via POST /api/v1/events
	evBody := `{"source_id":"src-1","external_event_id":"ext-999","event_type":"user.signup","payload":{"user_id":"u-1"}}`
	reqEv := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(evBody))
	reqEv.Header.Set("Content-Type", "application/json")
	wEv := httptest.NewRecorder()
	handler.ServeHTTP(wEv, reqEv)
	if wEv.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for event ingest, got %d: %s", wEv.Code, wEv.Body.String())
	}
	var ingestResp struct {
		Event *event.Event `json:"event"`
	}
	_ = json.Unmarshal(wEv.Body.Bytes(), &ingestResp)
	if ingestResp.Event == nil {
		t.Fatalf("expected event in ingest response: %s", wEv.Body.String())
	}
	ingested := ingestResp.Event
	// Save into events store for list/get queries
	_, _, _ = cfg.Events.Create(ctx, ingested)

	// List Events
	reqEvList := httptest.NewRequest(http.MethodGet, "/api/v1/events?source_id=src-1&event_type=user.signup&limit=10&offset=0", nil)
	wEvList := httptest.NewRecorder()
	handler.ServeHTTP(wEvList, reqEvList)
	if wEvList.Code != http.StatusOK {
		t.Fatalf("expected 200 for list events, got %d", wEvList.Code)
	}

	// Get Event
	reqEvGet := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+ingested.ID, nil)
	wEvGet := httptest.NewRecorder()
	handler.ServeHTTP(wEvGet, reqEvGet)
	if wEvGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for get event, got %d", wEvGet.Code)
	}

	// Seed Delivery Job
	job := delivery.NewJob(ingested.ID, "dest-1", 3)
	job.Status = delivery.StatusFailed
	job.Attempts = 3
	if err := cfg.Deliveries.CreateJob(ctx, job); err != nil {
		t.Fatalf("failed to seed delivery job: %v", err)
	}

	// List Deliveries
	reqDelivList := httptest.NewRequest(http.MethodGet, "/deliveries?status=failed&destination_id=dest-1&limit=5", nil)
	wDelivList := httptest.NewRecorder()
	handler.ServeHTTP(wDelivList, reqDelivList)
	if wDelivList.Code != http.StatusOK {
		t.Fatalf("expected 200 for deliveries, got %d", wDelivList.Code)
	}

	// List Failed Deliveries
	reqFailed := httptest.NewRequest(http.MethodGet, "/deliveries/failed", nil)
	wFailed := httptest.NewRecorder()
	handler.ServeHTTP(wFailed, reqFailed)
	if wFailed.Code != http.StatusOK {
		t.Fatalf("expected 200 for failed deliveries, got %d", wFailed.Code)
	}

	// Delivery Stats
	reqStats := httptest.NewRequest(http.MethodGet, "/deliveries/stats", nil)
	wStats := httptest.NewRecorder()
	handler.ServeHTTP(wStats, reqStats)
	if wStats.Code != http.StatusOK {
		t.Fatalf("expected 200 for delivery stats, got %d", wStats.Code)
	}

	// Get Delivery Job
	reqJobGet := httptest.NewRequest(http.MethodGet, "/deliveries/"+job.ID, nil)
	wJobGet := httptest.NewRecorder()
	handler.ServeHTTP(wJobGet, reqJobGet)
	if wJobGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for get delivery job, got %d", wJobGet.Code)
	}

	// Retry Delivery Job
	reqJobRetry := httptest.NewRequest(http.MethodPost, "/deliveries/"+job.ID+"/retry", nil)
	wJobRetry := httptest.NewRecorder()
	handler.ServeHTTP(wJobRetry, reqJobRetry)
	if wJobRetry.Code != http.StatusOK {
		t.Fatalf("expected 200 for retry delivery job, got %d", wJobRetry.Code)
	}
}
