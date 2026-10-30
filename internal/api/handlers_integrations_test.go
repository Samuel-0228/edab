package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIntegrationsAndStaticAssets(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	// List Integrations Catalog
	reqInt := httptest.NewRequest(http.MethodGet, "/api/v1/integrations", nil)
	wInt := httptest.NewRecorder()
	handler.ServeHTTP(wInt, reqInt)
	if wInt.Code != http.StatusOK {
		t.Fatalf("expected 200 for integrations, got %d", wInt.Code)
	}

	// Get Integration by ID
	reqIntGet := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/telegram", nil)
	wIntGet := httptest.NewRecorder()
	handler.ServeHTTP(wIntGet, reqIntGet)
	if wIntGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for get telegram integration, got %d", wIntGet.Code)
	}

	// Get Integration Schema
	reqSchema := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/telegram/schema", nil)
	wSchema := httptest.NewRecorder()
	handler.ServeHTTP(wSchema, reqSchema)
	if wSchema.Code != http.StatusOK {
		t.Fatalf("expected 200 for get telegram schema, got %d", wSchema.Code)
	}

	// Get Integration Schema with action_type query
	reqSchemaAction := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/telegram/schema?action_type=copy_message", nil)
	wSchemaAction := httptest.NewRecorder()
	handler.ServeHTTP(wSchemaAction, reqSchemaAction)
	if wSchemaAction.Code != http.StatusOK {
		t.Fatalf("expected 200 for get telegram copy_message schema, got %d", wSchemaAction.Code)
	}

	// Get Integration Schema not found
	reqSchema404 := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/unknown_xyz/schema", nil)
	wSchema404 := httptest.NewRecorder()
	handler.ServeHTTP(wSchema404, reqSchema404)
	if wSchema404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown integration schema, got %d", wSchema404.Code)
	}

	// Dashboard UI Assets
	reqUI := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	wUI := httptest.NewRecorder()
	handler.ServeHTTP(wUI, reqUI)
	if wUI.Code != http.StatusOK {
		t.Fatalf("expected 200 for UI, got %d", wUI.Code)
	}

	reqLogo := httptest.NewRequest(http.MethodGet, "/kerob_logo.png", nil)
	wLogo := httptest.NewRecorder()
	handler.ServeHTTP(wLogo, reqLogo)
	if wLogo.Code != http.StatusOK {
		t.Fatalf("expected 200 for logo, got %d", wLogo.Code)
	}
}
