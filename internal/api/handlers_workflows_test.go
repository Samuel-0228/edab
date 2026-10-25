package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/routing"
	"kerob/internal/core/workflow"
)

func TestAdvancedCRUDAndEdgeCases(t *testing.T) {
	server, handlers, cfg := setupTestServer()
	handler := server.Handler()
	ctx := context.Background()

	// 1. Create a workflow
	wf, _ := cfg.Workflows.Create(ctx, &workflow.Workflow{
		Name:    "Original Workflow",
		Enabled: true,
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{Integration: "discord", ActionType: "send_message", Config: json.RawMessage(`{"content":"hello"}`)},
		},
	})

	// PUT /api/v1/workflows/{id}
	putWF := `{"name":"Updated Workflow Title","trigger":{"type":"webhook"},"actions":[{"integration":"discord","action_type":"send_message","config":{"content":"updated"}}]}`
	reqPutWF := httptest.NewRequest(http.MethodPut, "/api/v1/workflows/"+wf.ID, strings.NewReader(putWF))
	wPutWF := httptest.NewRecorder()
	handler.ServeHTTP(wPutWF, reqPutWF)
	if wPutWF.Code != http.StatusOK {
		t.Fatalf("expected 200 for PUT workflow, got %d: %s", wPutWF.Code, wPutWF.Body.String())
	}

	// PUT /api/v1/workflows/{id} invalid validation (missing name)
	reqPutWFBad := httptest.NewRequest(http.MethodPut, "/api/v1/workflows/"+wf.ID, strings.NewReader(`{"name":""}`))
	wPutWFBad := httptest.NewRecorder()
	handler.ServeHTTP(wPutWFBad, reqPutWFBad)
	if wPutWFBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad PUT workflow, got %d", wPutWFBad.Code)
	}

	// PUT /api/v1/workflows/non-existent
	reqPutWF404 := httptest.NewRequest(http.MethodPut, "/api/v1/workflows/non-existent", strings.NewReader(putWF))
	wPutWF404 := httptest.NewRecorder()
	handler.ServeHTTP(wPutWF404, reqPutWF404)
	if wPutWF404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for PUT non-existent workflow, got %d", wPutWF404.Code)
	}

	// PATCH /api/v1/workflows/{id} with empty name -> validation error
	reqPatchWFBad := httptest.NewRequest(http.MethodPatch, "/api/v1/workflows/"+wf.ID, strings.NewReader(`{"name":"   "}`))
	wPatchWFBad := httptest.NewRecorder()
	handler.ServeHTTP(wPatchWFBad, reqPatchWFBad)
	if wPatchWFBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty patch name, got %d", wPatchWFBad.Code)
	}

	// PATCH /api/v1/workflows/non-existent -> 404
	reqPatchWF404 := httptest.NewRequest(http.MethodPatch, "/api/v1/workflows/non-existent", strings.NewReader(`{"name":"hello"}`))
	wPatchWF404 := httptest.NewRecorder()
	handler.ServeHTTP(wPatchWF404, reqPatchWF404)
	if wPatchWF404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for patch non-existent, got %d", wPatchWF404.Code)
	}

	// 2. Sources: Create, Update, Patch with username, Sync, IngestRaw
	src, _ := cfg.Sources.Create(ctx, &sources.Source{
		Name:        "Test Inbound Source",
		AdapterType: "telegram",
		Enabled:     true,
	})

	// PUT /sources/{id}
	putSrc := `{"name":"Renamed Source","adapter_type":"telegram","enabled":true}`
	reqPutSrc := httptest.NewRequest(http.MethodPut, "/sources/"+src.ID, strings.NewReader(putSrc))
	wPutSrc := httptest.NewRecorder()
	handler.ServeHTTP(wPutSrc, reqPutSrc)
	if wPutSrc.Code != http.StatusOK {
		t.Fatalf("expected 200 for PUT source, got %d: %s", wPutSrc.Code, wPutSrc.Body.String())
	}

	// PUT /sources/non-existent -> 404
	reqPutSrc404 := httptest.NewRequest(http.MethodPut, "/sources/non-existent", strings.NewReader(putSrc))
	wPutSrc404 := httptest.NewRecorder()
	handler.ServeHTTP(wPutSrc404, reqPutSrc404)
	if wPutSrc404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for PUT non-existent source, got %d", wPutSrc404.Code)
	}

	// PATCH /sources/{id} with ChannelUsername
	patchSrc := `{"channel_username":"@test_channel"}`
	reqPatchSrc := httptest.NewRequest(http.MethodPatch, "/sources/"+src.ID, strings.NewReader(patchSrc))
	wPatchSrc := httptest.NewRecorder()
	handler.ServeHTTP(wPatchSrc, reqPatchSrc)
	if wPatchSrc.Code != http.StatusOK {
		t.Fatalf("expected 200 for PATCH source username, got %d", wPatchSrc.Code)
	}

	// Sync Source (enabled -> 200)
	reqSync := httptest.NewRequest(http.MethodPost, "/sources/"+src.ID+"/sync", nil)
	wSync := httptest.NewRecorder()
	handler.ServeHTTP(wSync, reqSync)
	if wSync.Code != http.StatusOK {
		t.Fatalf("expected 200 for sync source, got %d", wSync.Code)
	}

	// Sync Source non-existent -> 404
	reqSync404 := httptest.NewRequest(http.MethodPost, "/sources/non-existent/sync", nil)
	wSync404 := httptest.NewRecorder()
	handler.ServeHTTP(wSync404, reqSync404)
	if wSync404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for sync non-existent source, got %d", wSync404.Code)
	}

	// 3. Destinations: Create, Update, Patch
	dest, _ := cfg.Destinations.Create(ctx, &destinations.Destination{
		Name:        "Test Outbound Dest",
		AdapterType: "discord",
		Enabled:     true,
	})

	// PUT /destinations/{id}
	putDest := `{"name":"Updated Dest","adapter_type":"discord","enabled":true}`
	reqPutDest := httptest.NewRequest(http.MethodPut, "/destinations/"+dest.ID, strings.NewReader(putDest))
	wPutDest := httptest.NewRecorder()
	handler.ServeHTTP(wPutDest, reqPutDest)
	if wPutDest.Code != http.StatusOK {
		t.Fatalf("expected 200 for PUT dest, got %d: %s", wPutDest.Code, wPutDest.Body.String())
	}

	// PUT /destinations/non-existent -> 404
	reqPutDest404 := httptest.NewRequest(http.MethodPut, "/destinations/non-existent", strings.NewReader(putDest))
	wPutDest404 := httptest.NewRecorder()
	handler.ServeHTTP(wPutDest404, reqPutDest404)
	if wPutDest404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for PUT non-existent dest, got %d", wPutDest404.Code)
	}

	// PATCH /destinations/{id} with config
	patchDest := `{"name":"Patched Dest","enabled":false}`
	reqPatchDest := httptest.NewRequest(http.MethodPatch, "/destinations/"+dest.ID, strings.NewReader(patchDest))
	wPatchDest := httptest.NewRecorder()
	handler.ServeHTTP(wPatchDest, reqPatchDest)
	if wPatchDest.Code != http.StatusOK {
		t.Fatalf("expected 200 for PATCH dest, got %d", wPatchDest.Code)
	}

	// 4. Routes: Create, Update
	rt, _ := cfg.Routes.Create(ctx, &routing.Route{
		SourceID:      src.ID,
		DestinationID: dest.ID,
		Enabled:       true,
	})

	// PUT /routes/{id}
	putRoute := fmt.Sprintf(`{"source_id":"%s","destination_id":"%s","enabled":false}`, src.ID, dest.ID)
	reqPutRoute := httptest.NewRequest(http.MethodPut, "/routes/"+rt.ID, strings.NewReader(putRoute))
	wPutRoute := httptest.NewRecorder()
	handler.ServeHTTP(wPutRoute, reqPutRoute)
	if wPutRoute.Code != http.StatusOK {
		t.Fatalf("expected 200 for PUT route, got %d: %s", wPutRoute.Code, wPutRoute.Body.String())
	}

	// PUT /routes/non-existent -> 404
	reqPutRoute404 := httptest.NewRequest(http.MethodPut, "/routes/non-existent", strings.NewReader(putRoute))
	wPutRoute404 := httptest.NewRecorder()
	handler.ServeHTTP(wPutRoute404, reqPutRoute404)
	if wPutRoute404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for PUT non-existent route, got %d", wPutRoute404.Code)
	}

	// Metrics & Setters
	RecordWorkflowExecution("success")
	RecordWorkflowExecution("failed")
	handlers.SetTelegramAPI(nil)
	handlers.SetWorkflowServices(cfg.Workflows, cfg.Executions, cfg.DeadLetters, cfg.WorkflowEngine, cfg.ActionReg, cfg.Creds, cfg.WebhookStore)
	_ = handlers.Tracker()
}
