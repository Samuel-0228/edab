package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/credentials"
	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/core/webhooks"
	"kerob/internal/core/workflow"
)

func setupTestServer() (*Server, *Handlers, Config) {
	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-32-byte-secret-key-1234567"))
	whStore := webhooks.NewMemoryStore()
	srcStore := sources.NewMemoryStore()
	destStore := destinations.NewMemoryStore()
	routeStore := routing.NewMemoryStore()
	eventStore := event.NewMemoryStore()
	deliveryStore := delivery.NewMemoryStore()

	srcReg := sources.NewRegistry()
	srcReg.Register("telegram", func() sources.SourceAdapter { return sources.NewMockSourceAdapter() })
	srcReg.Register("github", func() sources.SourceAdapter { return sources.NewMockSourceAdapter() })

	destReg := destinations.NewRegistry()
	destReg.Register("discord", func() destinations.DestinationAdapter { return destinations.NewMockDestinationAdapter() })
	destReg.Register("telegram", func() destinations.DestinationAdapter { return destinations.NewMockDestinationAdapter() })

	actionReg := workflow.NewActionRegistry()
	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, actionReg)

	cfg := Config{
		Workflows:      wfStore,
		Executions:     execStore,
		DeadLetters:    dlStore,
		WorkflowEngine: engine,
		ActionReg:      actionReg,
		Creds:          credStore,
		WebhookStore:   whStore,
		Sources:        srcStore,
		Destinations:   destStore,
		Routes:         routeStore,
		Events:         eventStore,
		Deliveries:     deliveryStore,
		SourceReg:      srcReg,
		DestReg:        destReg,
	}

	handlers := NewHandlersFromConfig(cfg)
	server := NewServer("0", handlers)
	return server, handlers, cfg
}

func TestExecutionsAPI(t *testing.T) {
	server, _, cfg := setupTestServer()
	handler := server.Handler()
	ctx := context.Background()

	// Seed executions
	exec1, err := cfg.Executions.Create(ctx, &workflow.Execution{
		WorkflowID:  "wf-1",
		EventID:     "ev-1",
		Status:      workflow.ExecutionSuccess,
		Attempts:    1,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("failed to seed execution: %v", err)
	}

	exec2, _ := cfg.Executions.Create(ctx, &workflow.Execution{
		WorkflowID:  "wf-2",
		EventID:     "ev-2",
		Status:      workflow.ExecutionFailed,
		Attempts:    3,
		MaxAttempts: 3,
	})

	// 1. List all executions
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/executions", nil)
	wList := httptest.NewRecorder()
	handler.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wList.Code)
	}
	var list []*workflow.Execution
	_ = json.Unmarshal(wList.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Errorf("expected 2 executions, got %d", len(list))
	}

	// 2. Filter executions by status
	reqFilter := httptest.NewRequest(http.MethodGet, "/api/v1/executions?status=failed&workflow_id=wf-2&limit=10&offset=0", nil)
	wFilter := httptest.NewRecorder()
	handler.ServeHTTP(wFilter, reqFilter)
	if wFilter.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wFilter.Code)
	}
	var filtered []*workflow.Execution
	_ = json.Unmarshal(wFilter.Body.Bytes(), &filtered)
	if len(filtered) != 1 || filtered[0].ID != exec2.ID {
		t.Errorf("expected filtered execution %s, got %+v", exec2.ID, filtered)
	}

	// 3. Get execution by ID
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/executions/"+exec1.ID, nil)
	wGet := httptest.NewRecorder()
	handler.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wGet.Code)
	}

	// 4. Get non-existent execution -> 404
	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/executions/non-existent-id", nil)
	w404 := httptest.NewRecorder()
	handler.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w404.Code)
	}

	// 5. Retry execution
	reqRetry := httptest.NewRequest(http.MethodPost, "/api/v1/executions/"+exec2.ID+"/retry", nil)
	wRetry := httptest.NewRecorder()
	handler.ServeHTTP(wRetry, reqRetry)
	if wRetry.Code != http.StatusOK {
		t.Fatalf("expected 200 for retry, got %d: %s", wRetry.Code, wRetry.Body.String())
	}

	// 6. Retry non-existent execution -> 404
	reqRetry404 := httptest.NewRequest(http.MethodPost, "/api/v1/executions/non-existent-id/retry", nil)
	wRetry404 := httptest.NewRecorder()
	handler.ServeHTTP(wRetry404, reqRetry404)
	if wRetry404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for retry non-existent, got %d", wRetry404.Code)
	}
}

func TestFailuresAPI(t *testing.T) {
	server, _, cfg := setupTestServer()
	handler := server.Handler()
	ctx := context.Background()

	// Seed an execution first so retry finds it
	_, err := cfg.Executions.Create(ctx, &workflow.Execution{
		ID:          "exec-1",
		WorkflowID:  "wf-1",
		EventID:     "ev-1",
		Status:      workflow.ExecutionFailed,
		Attempts:    3,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("failed to seed execution: %v", err)
	}

	// Seed dead-letter record
	rec, err := cfg.DeadLetters.Record(ctx, &workflow.DeadLetterRecord{
		ExecutionID: "exec-1",
		WorkflowID:  "wf-1",
		EventID:     "ev-1",
		Error:       "connection reset by peer",
	})
	if err != nil {
		t.Fatalf("failed to record dead letter: %v", err)
	}

	// 1. List failures
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/failures", nil)
	wList := httptest.NewRecorder()
	handler.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wList.Code)
	}
	var list []*workflow.DeadLetterRecord
	_ = json.Unmarshal(wList.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 failure record, got %d", len(list))
	}

	// 2. Get failure by ID
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/failures/"+rec.ID, nil)
	wGet := httptest.NewRecorder()
	handler.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wGet.Code)
	}

	// 3. Get non-existent failure -> 404
	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/failures/non-existent-id", nil)
	w404 := httptest.NewRecorder()
	handler.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w404.Code)
	}

	// 4. Retry failure
	reqRetry := httptest.NewRequest(http.MethodPost, "/api/v1/failures/"+rec.ID+"/retry", nil)
	wRetry := httptest.NewRecorder()
	handler.ServeHTTP(wRetry, reqRetry)
	if wRetry.Code != http.StatusOK {
		t.Fatalf("expected 200 for retry failure, got %d: %s", wRetry.Code, wRetry.Body.String())
	}

	// 5. Discard failure
	reqDiscard := httptest.NewRequest(http.MethodPost, "/api/v1/failures/"+rec.ID+"/discard", nil)
	wDiscard := httptest.NewRecorder()
	handler.ServeHTTP(wDiscard, reqDiscard)
	if wDiscard.Code != http.StatusOK {
		t.Fatalf("expected 200 for discard failure, got %d: %s", wDiscard.Code, wDiscard.Body.String())
	}

	// 6. Discard non-existent failure -> 404
	reqDiscard404 := httptest.NewRequest(http.MethodPost, "/api/v1/failures/non-existent-id/discard", nil)
	wDiscard404 := httptest.NewRecorder()
	handler.ServeHTTP(wDiscard404, reqDiscard404)
	if wDiscard404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", wDiscard404.Code)
	}
}

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
		t.Fatalf("expected 200, got %d: %s", wUpdate.Code, wUpdate.Body.String())
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

	_ = time.Second
	_ = bytes.Buffer{}
}

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

func TestNilStoreGuards(t *testing.T) {
	// Handlers with empty/nil stores must handle requests gracefully without panic
	emptyHandlers := &Handlers{}
	server := NewServer("0", emptyHandlers)
	handler := server.Handler()

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/workflows"},
		{http.MethodGet, "/api/v1/executions"},
		{http.MethodGet, "/api/v1/executions/123"},
		{http.MethodPost, "/api/v1/executions/123/retry"},
		{http.MethodGet, "/api/v1/failures"},
		{http.MethodGet, "/api/v1/failures/123"},
		{http.MethodPost, "/api/v1/failures/123/retry"},
		{http.MethodPost, "/api/v1/failures/123/discard"},
		{http.MethodGet, "/api/v1/webhooks"},
		{http.MethodGet, "/api/v1/webhooks/123"},
		{http.MethodGet, "/api/v1/credentials"},
		{http.MethodGet, "/api/v1/credentials/123"},
		{http.MethodGet, "/api/v1/events"},
		{http.MethodGet, "/api/v1/events/123"},
		{http.MethodGet, "/sources"},
		{http.MethodGet, "/sources/123"},
		{http.MethodGet, "/destinations"},
		{http.MethodGet, "/destinations/123"},
		{http.MethodGet, "/routes"},
		{http.MethodGet, "/routes/123"},
		{http.MethodGet, "/deliveries"},
		{http.MethodGet, "/deliveries/failed"},
		{http.MethodGet, "/deliveries/stats"},
		{http.MethodGet, "/deliveries/123"},
		{http.MethodPost, "/deliveries/123/retry"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
			t.Errorf("expected 200 or 500 for %s %s with nil store, got %d", ep.method, ep.path, w.Code)
		}
	}
}
