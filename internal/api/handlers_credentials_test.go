package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerob/internal/core/credentials"
)

func TestCredentialsCRUD(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	// 1. Create Credential
	createBody := `{"name":"Discord Prod","integration":"discord","values":{"token":"bot-token-12345"}}`
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/credentials", strings.NewReader(createBody))
	reqCreate.Header.Set("Content-Type", "application/json")
	wCreate := httptest.NewRecorder()
	handler.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", wCreate.Code, wCreate.Body.String())
	}
	var created credentials.Credential
	_ = json.Unmarshal(wCreate.Body.Bytes(), &created)

	// 2. List Credentials
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/credentials", nil)
	wList := httptest.NewRecorder()
	handler.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wList.Code)
	}

	// 3. Get Credential
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/credentials/"+created.ID, nil)
	wGet := httptest.NewRecorder()
	handler.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wGet.Code)
	}

	// 4. Update Credential
	updateBody := `{"name":"Discord Staging","integration":"discord","values":{"token":"bot-token-99999"}}`
	reqUpdate := httptest.NewRequest(http.MethodPut, "/api/v1/credentials/"+created.ID, strings.NewReader(updateBody))
	reqUpdate.Header.Set("Content-Type", "application/json")
	wUpdate := httptest.NewRecorder()
	handler.ServeHTTP(wUpdate, reqUpdate)
	if wUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wUpdate.Code)
	}

	// 5. Delete Credential
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/credentials/"+created.ID, nil)
	wDel := httptest.NewRecorder()
	handler.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", wDel.Code)
	}

	// 6. Get deleted -> 404
	reqGetDel := httptest.NewRequest(http.MethodGet, "/api/v1/credentials/"+created.ID, nil)
	wGetDel := httptest.NewRecorder()
	handler.ServeHTTP(wGetDel, reqGetDel)
	if wGetDel.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", wGetDel.Code)
	}
}
