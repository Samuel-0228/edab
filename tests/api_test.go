package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/api"
	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/telegram"
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

func TestAPITelegramAndManagementEndpoints(t *testing.T) {
	evStore, srcStore, destStore, routeStore, jobStore, srcReg, destReg, _, pipe := setupTestEnvironment()
	mockAPI := telegram.NewMockAPI()

	handlers := api.NewHandlers(srcStore, destStore, routeStore, evStore, jobStore, pipe, srcReg, destReg)
	handlers.SetTelegramAPI(mockAPI)
	server := api.NewServer("0", handlers)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := ts.Client()

	// 1. Create Telegram Source using convenience format
	srcPayload := []byte(`{"name": "Channel Alpha", "telegram_chat_id": "-100123456"}`)
	resp, err := client.Post(ts.URL+"/sources", "application/json", bytes.NewReader(srcPayload))
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("convenience POST /sources failed: status=%d err=%v", resp.StatusCode, err)
	}
	var src sources.Source
	_ = json.NewDecoder(resp.Body).Decode(&src)
	if src.AdapterType != "telegram" {
		t.Fatalf("expected adapter_type 'telegram', got %s", src.AdapterType)
	}

	// 2. Check Source Access (GET /sources/{id}/access)
	resp, err = client.Get(ts.URL + "/sources/" + src.ID + "/access")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /sources/{id}/access failed: status=%d err=%v", resp.StatusCode, err)
	}
	var accessInfo telegram.AccessInfo
	_ = json.NewDecoder(resp.Body).Decode(&accessInfo)
	if !accessInfo.Accessible {
		t.Fatalf("expected accessible=true, got %+v", accessInfo)
	}

	// 3. PATCH /sources/{id} to toggle enabled
	patchSrc := []byte(`{"enabled": false}`)
	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/sources/"+src.ID, bytes.NewReader(patchSrc))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /sources/{id} failed: status=%d err=%v", resp.StatusCode, err)
	}
	_ = json.NewDecoder(resp.Body).Decode(&src)
	if src.Enabled {
		t.Fatal("expected source to be disabled after PATCH")
	}

	// 4. Create Telegram Destination using convenience format
	destPayload := []byte(`{"name": "Backup Target", "telegram_chat_id": "-100987654"}`)
	resp, err = client.Post(ts.URL+"/destinations", "application/json", bytes.NewReader(destPayload))
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("convenience POST /destinations failed: status=%d err=%v", resp.StatusCode, err)
	}
	var dest destinations.Destination
	_ = json.NewDecoder(resp.Body).Decode(&dest)
	if dest.AdapterType != "telegram" {
		t.Fatalf("expected adapter_type 'telegram', got %s", dest.AdapterType)
	}

	// 5. Check Destination Access (GET /destinations/{id}/access)
	resp, err = client.Get(ts.URL + "/destinations/" + dest.ID + "/access")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /destinations/{id}/access failed: status=%d err=%v", resp.StatusCode, err)
	}

	// 6. Create Route
	routeBody, _ := json.Marshal(map[string]any{
		"source_id":      src.ID,
		"destination_id": dest.ID,
		"enabled":        true,
	})
	resp, err = client.Post(ts.URL+"/routes", "application/json", bytes.NewReader(routeBody))
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /routes failed: status=%d", resp.StatusCode)
	}
	var route routing.Route
	_ = json.NewDecoder(resp.Body).Decode(&route)

	// 7. PATCH /routes/{id} to disable
	patchRoute := []byte(`{"enabled": false}`)
	req, _ = http.NewRequest(http.MethodPatch, ts.URL+"/routes/"+route.ID, bytes.NewReader(patchRoute))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /routes/{id} failed: status=%d", resp.StatusCode)
	}
	_ = json.NewDecoder(resp.Body).Decode(&route)
	if route.Enabled {
		t.Fatal("expected route to be disabled after PATCH")
	}

	// 8. Delivery Stats (GET /deliveries/stats)
	resp, err = client.Get(ts.URL + "/deliveries/stats")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /deliveries/stats failed: status=%d", resp.StatusCode)
	}
	var stats delivery.Stats
	_ = json.NewDecoder(resp.Body).Decode(&stats)

	// 9. Failed Deliveries (GET /deliveries/failed)
	resp, err = client.Get(ts.URL + "/deliveries/failed")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /deliveries/failed failed: status=%d", resp.StatusCode)
	}

	// 10. Dashboard UI (GET /, GET /ui, and GET /ui/{path...})
	resp, err = client.Get(ts.URL + "/")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / failed: status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "Kerob Engine Control") || !strings.Contains(string(body), `id="app"`) {
		t.Fatalf("GET / did not serve Svelte dashboard application")
	}

	resp, err = client.Get(ts.URL + "/ui")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui failed: status=%d", resp.StatusCode)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "Kerob Engine Control") {
		t.Fatalf("GET /ui did not serve Svelte dashboard application")
	}

	resp, err = client.Get(ts.URL + "/ui/workflows")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/workflows failed: status=%d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// 11. Static assets (logo and favicon)
	resp, err = client.Get(ts.URL + "/kerob_logo.png")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /kerob_logo.png failed: status=%d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("GET /kerob_logo.png invalid Content-Type: %s", ct)
	}
	_ = resp.Body.Close()

	resp, err = client.Get(ts.URL + "/favicon.ico")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /favicon.ico failed: status=%d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("GET /favicon.ico invalid Content-Type: %s", ct)
	}
	_ = resp.Body.Close()
}
