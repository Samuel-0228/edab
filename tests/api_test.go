package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-engine/internal/adapters/destinations"
	"automation-engine/internal/adapters/sources"
	"automation-engine/internal/api"
	"automation-engine/internal/core/delivery"
	"automation-engine/internal/core/event"
	"automation-engine/internal/core/routing"
)

func setupAPITestServer() (*httptest.Server, *MemoryEventStore, *MemorySourceStore, *MemoryDestinationStore, *MemoryRouteStore, *MemoryDeliveryStore) {
	evStore, srcStore, destStore, routeStore, jobStore, srcReg, destReg, _, pipe := setupTestEnvironment()

	handlers := api.NewHandlers(srcStore, destStore, routeStore, evStore, jobStore, pipe, srcReg, destReg)
	server := api.NewServer("0", handlers)
	ts := httptest.NewServer(server.Handler())
	return ts, evStore, srcStore, destStore, routeStore, jobStore
}

func TestAPIEndpoints(t *testing.T) {
	ts, _, _, _, _, _ := setupAPITestServer()
	defer ts.Close()

	client := ts.Client()

	// 1. Health check
	resp, err := client.Get(ts.URL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health failed: status=%d err=%v", resp.StatusCode, err)
	}

	// 2. Ready check
	resp, err = client.Get(ts.URL + "/ready")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ready failed: status=%d err=%v", resp.StatusCode, err)
	}

	// 3. Create Source (POST /sources)
	sourceBody := []byte(`{"name": "API Webhook", "adapter_type": "webhook", "enabled": true}`)
	resp, err = client.Post(ts.URL+"/sources", "application/json", bytes.NewReader(sourceBody))
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /sources failed: status=%d err=%v", resp.StatusCode, err)
	}
	var createdSrc sources.Source
	_ = json.NewDecoder(resp.Body).Decode(&createdSrc)
	if createdSrc.ID == "" || createdSrc.Name != "API Webhook" {
		t.Fatalf("unexpected source created: %+v", createdSrc)
	}

	// 4. Get Source (GET /sources/{id})
	resp, err = client.Get(ts.URL + "/sources/" + createdSrc.ID)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /sources/{id} failed: status=%d err=%v", resp.StatusCode, err)
	}

	// 5. List Sources (GET /sources)
	resp, err = client.Get(ts.URL + "/sources")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /sources failed: status=%d err=%v", resp.StatusCode, err)
	}

	// 6. Create Destination (POST /destinations)
	destBody := []byte(`{"name": "API Mock Dest", "adapter_type": "mock", "enabled": true}`)
	resp, err = client.Post(ts.URL+"/destinations", "application/json", bytes.NewReader(destBody))
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /destinations failed: status=%d err=%v", resp.StatusCode, err)
	}
	var createdDest destinations.Destination
	_ = json.NewDecoder(resp.Body).Decode(&createdDest)
	if createdDest.ID == "" {
		t.Fatalf("destination ID is empty")
	}

	// 7. Create Route (POST /routes)
	routeBody, _ := json.Marshal(map[string]any{
		"source_id":      createdSrc.ID,
		"destination_id": createdDest.ID,
		"enabled":        true,
	})
	resp, err = client.Post(ts.URL+"/routes", "application/json", bytes.NewReader(routeBody))
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /routes failed: status=%d err=%v", resp.StatusCode, err)
	}
	var createdRoute routing.Route
	_ = json.NewDecoder(resp.Body).Decode(&createdRoute)

	// 8. Ingest raw event via Source Ingest endpoint (POST /sources/{id}/ingest)
	ingestBody := []byte(`{"id": "api-ext-1", "type": "webhook.event", "payload": "hello world"}`)
	resp, err = client.Post(ts.URL+"/sources/"+createdSrc.ID+"/ingest", "application/json", bytes.NewReader(ingestBody))
	if err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /sources/{id}/ingest failed: status=%d err=%v", resp.StatusCode, err)
	}
	var ingestedEv event.Event
	_ = json.NewDecoder(resp.Body).Decode(&ingestedEv)
	if ingestedEv.ID == "" || ingestedEv.ExternalEventID != "api-ext-1" {
		t.Errorf("unexpected ingested event: %+v", ingestedEv)
	}

	// 9. List Events (GET /events)
	resp, err = client.Get(ts.URL + "/events")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /events failed: status=%d", resp.StatusCode)
	}
	var events []*event.Event
	_ = json.NewDecoder(resp.Body).Decode(&events)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	// 10. List Deliveries (GET /deliveries)
	resp, err = client.Get(ts.URL + "/deliveries")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /deliveries failed: status=%d", resp.StatusCode)
	}
	var deliveries []*delivery.DeliveryJob
	_ = json.NewDecoder(resp.Body).Decode(&deliveries)
	if len(deliveries) != 1 {
		t.Errorf("expected 1 delivery job, got %d", len(deliveries))
	}

	// 11. Retry Delivery endpoint (POST /deliveries/{id}/retry)
	resp, err = client.Post(ts.URL+"/deliveries/"+deliveries[0].ID+"/retry", "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /deliveries/{id}/retry failed: status=%d", resp.StatusCode)
	}
}
