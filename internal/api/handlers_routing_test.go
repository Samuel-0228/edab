package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/routing"
)

func TestSourcesAndDestinationsAndRoutes(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	// 1. Create Source
	srcBody := `{"name":"Telegram Inbound","adapter_type":"telegram","config":{"chat_id":-100123}}`
	reqSrc := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(srcBody))
	reqSrc.Header.Set("Content-Type", "application/json")
	wSrc := httptest.NewRecorder()
	handler.ServeHTTP(wSrc, reqSrc)
	if wSrc.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for source, got %d: %s", wSrc.Code, wSrc.Body.String())
	}
	var src sources.Source
	_ = json.Unmarshal(wSrc.Body.Bytes(), &src)

	// List Sources
	reqSrcList := httptest.NewRequest(http.MethodGet, "/sources", nil)
	wSrcList := httptest.NewRecorder()
	handler.ServeHTTP(wSrcList, reqSrcList)
	if wSrcList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wSrcList.Code)
	}

	// Get Source
	reqSrcGet := httptest.NewRequest(http.MethodGet, "/sources/"+src.ID, nil)
	wSrcGet := httptest.NewRecorder()
	handler.ServeHTTP(wSrcGet, reqSrcGet)
	if wSrcGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wSrcGet.Code)
	}

	// Patch Source (toggle status)
	patchSrcBody := `{"status":"disabled"}`
	reqSrcPatch := httptest.NewRequest(http.MethodPatch, "/sources/"+src.ID, strings.NewReader(patchSrcBody))
	wSrcPatch := httptest.NewRecorder()
	handler.ServeHTTP(wSrcPatch, reqSrcPatch)
	if wSrcPatch.Code != http.StatusOK {
		t.Fatalf("expected 200 for patch source, got %d: %s", wSrcPatch.Code, wSrcPatch.Body.String())
	}

	// Check Source Access
	reqSrcAccess := httptest.NewRequest(http.MethodGet, "/sources/"+src.ID+"/access", nil)
	wSrcAccess := httptest.NewRecorder()
	handler.ServeHTTP(wSrcAccess, reqSrcAccess)
	if wSrcAccess.Code != http.StatusOK {
		t.Fatalf("expected 200 for source access, got %d", wSrcAccess.Code)
	}

	// 2. Create Destination
	destBody := `{"name":"Discord Channel","adapter_type":"discord","config":{"channel_id":"12345"}}`
	reqDest := httptest.NewRequest(http.MethodPost, "/destinations", strings.NewReader(destBody))
	reqDest.Header.Set("Content-Type", "application/json")
	wDest := httptest.NewRecorder()
	handler.ServeHTTP(wDest, reqDest)
	if wDest.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for destination, got %d: %s", wDest.Code, wDest.Body.String())
	}
	var dest destinations.Destination
	_ = json.Unmarshal(wDest.Body.Bytes(), &dest)

	// List Destinations
	reqDestList := httptest.NewRequest(http.MethodGet, "/destinations", nil)
	wDestList := httptest.NewRecorder()
	handler.ServeHTTP(wDestList, reqDestList)
	if wDestList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wDestList.Code)
	}

	// Get Destination
	reqDestGet := httptest.NewRequest(http.MethodGet, "/destinations/"+dest.ID, nil)
	wDestGet := httptest.NewRecorder()
	handler.ServeHTTP(wDestGet, reqDestGet)
	if wDestGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wDestGet.Code)
	}

	// Check Destination Access
	reqDestAccess := httptest.NewRequest(http.MethodGet, "/destinations/"+dest.ID+"/access", nil)
	wDestAccess := httptest.NewRecorder()
	handler.ServeHTTP(wDestAccess, reqDestAccess)
	if wDestAccess.Code != http.StatusOK {
		t.Fatalf("expected 200 for destination access, got %d", wDestAccess.Code)
	}

	// 3. Create Route connecting Source and Destination
	routeBody := fmt.Sprintf(`{"source_id":"%s","destination_id":"%s","status":"active"}`, src.ID, dest.ID)
	reqRoute := httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(routeBody))
	reqRoute.Header.Set("Content-Type", "application/json")
	wRoute := httptest.NewRecorder()
	handler.ServeHTTP(wRoute, reqRoute)
	if wRoute.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for route, got %d: %s", wRoute.Code, wRoute.Body.String())
	}
	var createdRoute routing.Route
	_ = json.Unmarshal(wRoute.Body.Bytes(), &createdRoute)

	// List Routes
	reqRouteList := httptest.NewRequest(http.MethodGet, "/routes", nil)
	wRouteList := httptest.NewRecorder()
	handler.ServeHTTP(wRouteList, reqRouteList)
	if wRouteList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wRouteList.Code)
	}

	// Get Route
	reqRouteGet := httptest.NewRequest(http.MethodGet, "/routes/"+createdRoute.ID, nil)
	wRouteGet := httptest.NewRecorder()
	handler.ServeHTTP(wRouteGet, reqRouteGet)
	if wRouteGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wRouteGet.Code)
	}

	// Patch Route
	patchRouteBody := `{"status":"paused"}`
	reqRoutePatch := httptest.NewRequest(http.MethodPatch, "/routes/"+createdRoute.ID, strings.NewReader(patchRouteBody))
	wRoutePatch := httptest.NewRecorder()
	handler.ServeHTTP(wRoutePatch, reqRoutePatch)
	if wRoutePatch.Code != http.StatusOK {
		t.Fatalf("expected 200 for patch route, got %d", wRoutePatch.Code)
	}

	// Delete Route
	reqRouteDel := httptest.NewRequest(http.MethodDelete, "/routes/"+createdRoute.ID, nil)
	wRouteDel := httptest.NewRecorder()
	handler.ServeHTTP(wRouteDel, reqRouteDel)
	if wRouteDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for delete route, got %d", wRouteDel.Code)
	}

	// Delete Destination
	reqDestDel := httptest.NewRequest(http.MethodDelete, "/destinations/"+dest.ID, nil)
	wDestDel := httptest.NewRecorder()
	handler.ServeHTTP(wDestDel, reqDestDel)
	if wDestDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for delete destination, got %d", wDestDel.Code)
	}

	// Delete Source
	reqSrcDel := httptest.NewRequest(http.MethodDelete, "/sources/"+src.ID, nil)
	wSrcDel := httptest.NewRecorder()
	handler.ServeHTTP(wSrcDel, reqSrcDel)
	if wSrcDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for delete source, got %d", wSrcDel.Code)
	}
}
