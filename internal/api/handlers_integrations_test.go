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
