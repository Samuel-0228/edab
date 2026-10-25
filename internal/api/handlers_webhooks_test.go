package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerob/internal/core/webhooks"
)

func TestWebhooksCRUD(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	// 1. Create Webhook
	createBody := `{"name":"Incoming Stripe","secret_token":"test-stripe-sec-123","description":"Stripe webhook","enabled":true}`
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(createBody))
	reqCreate.Header.Set("Content-Type", "application/json")
	wCreate := httptest.NewRecorder()
	handler.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", wCreate.Code, wCreate.Body.String())
	}
	var created webhooks.Webhook
	_ = json.Unmarshal(wCreate.Body.Bytes(), &created)
	if created.ID == "" || created.SecretToken == "" {
		t.Fatalf("expected created webhook with ID and SecretToken, got: %+v", created)
	}

	// 2. List Webhooks
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
	wList := httptest.NewRecorder()
	handler.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wList.Code)
	}

	// 3. Get Webhook
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+created.ID, nil)
	wGet := httptest.NewRecorder()
	handler.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wGet.Code)
	}

	// 4. Update Webhook
	updateBody := `{"name":"Updated Stripe Webhook","description":"New desc","enabled":true}`
	reqUpdate := httptest.NewRequest(http.MethodPut, "/api/v1/webhooks/"+created.ID, strings.NewReader(updateBody))
	reqUpdate.Header.Set("Content-Type", "application/json")
	wUpdate := httptest.NewRecorder()
	handler.ServeHTTP(wUpdate, reqUpdate)
	if wUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wUpdate.Code)
	}

	// 5. Ingest Webhook (POST /api/v1/webhooks/{id}/ingest)
	reqIngest := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+created.ID+"/ingest", strings.NewReader(`{"charge":"succeeded"}`))
	reqIngest.Header.Set("X-Webhook-Token", created.SecretToken)
	wIngest := httptest.NewRecorder()
	handler.ServeHTTP(wIngest, reqIngest)
	if wIngest.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", wIngest.Code, wIngest.Body.String())
	}

	// 6. Delete Webhook
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/webhooks/"+created.ID, nil)
	wDel := httptest.NewRecorder()
	handler.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", wDel.Code)
	}

	// 7. Get deleted -> 404
	reqGetDeleted := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+created.ID, nil)
	wGetDeleted := httptest.NewRecorder()
	handler.ServeHTTP(wGetDeleted, reqGetDeleted)
	if wGetDeleted.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", wGetDeleted.Code)
	}
}
