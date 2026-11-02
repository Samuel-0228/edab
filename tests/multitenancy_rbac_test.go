package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerob/internal/adapters/actions"
	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/api"
	"kerob/internal/core/auth"
	"kerob/internal/core/credentials"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/core/webhooks"
	"kerob/internal/core/workflow"
)

type multiTenantEnv struct {
	server      *httptest.Server
	authStore   auth.Store
	wfStore     workflow.Store
	execStore   workflow.ExecutionStore
	dlStore     workflow.DeadLetterStore
	credStore   credentials.Store
	whStore     webhooks.Store
	eventStore  event.Store
	tokenAlpha  string
	tokenBeta   string
	tokenViewer string
	tokenScoped string
}

func createTestAPIToken(ctx context.Context, store auth.Store, tenantID, userID, name string, role auth.Role, scopes []string) (string, error) {
	rawToken, prefix, hash, err := auth.GenerateAPIToken("live")
	if err != nil {
		return "", err
	}
	tok := &auth.APIToken{
		TenantID:    tenantID,
		Name:        name,
		TokenPrefix: prefix,
		TokenHash:   hash,
		Role:        role,
		Scopes:      scopes,
		CreatedBy:   userID,
	}
	_, err = store.CreateAPIToken(ctx, tok)
	return rawToken, err
}

func setupMultiTenantTestServer(t *testing.T) *multiTenantEnv {
	t.Helper()

	authStore := auth.NewMemoryStore()
	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore([]byte("01234567890123456789012345678901"))
	whStore := webhooks.NewMemoryStore()
	eventStore := NewMemoryEventStore()
	sourceStore := NewMemorySourceStore()
	destStore := NewMemoryDestinationStore()
	routeStore := NewMemoryRouteStore()
	delivStore := NewMemoryDeliveryStore()

	actionReg := workflow.NewActionRegistry()
	actionReg.Register(actions.NewHTTPActionExecutor())
	wfEngine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, actionReg, workflow.EngineConfig{})

	sourceReg := sources.NewRegistry()
	destReg := destinations.NewRegistry()
	pipeline := workflow.NewPipeline(sourceStore, sourceReg, eventStore, routing.NewRoutingEngine(routeStore), delivStore, destStore, 3)

	handlers := api.NewHandlersFromConfig(api.Config{
		Workflows:      wfStore,
		Executions:     execStore,
		DeadLetters:    dlStore,
		WorkflowEngine: wfEngine,
		ActionReg:      actionReg,
		Creds:          credStore,
		WebhookStore:   whStore,
		Sources:        sourceStore,
		Destinations:   destStore,
		Routes:         routeStore,
		Events:         eventStore,
		Deliveries:     delivStore,
		Pipeline:       pipeline,
		SourceReg:      sourceReg,
		DestReg:        destReg,
		AuthStore:      authStore,
		MultiTenancy:   true,
	})

	server := api.NewServerWithOptions("0", handlers, api.ServerOptions{
		AuthStore:           authStore,
		MultiTenancyEnabled: true,
		DefaultTenantID:     "default",
	})
	ts := httptest.NewServer(server.Handler())

	ctx := context.Background()

	// 1. Create Tenant Alpha
	tenantAlpha, err := authStore.CreateTenant(ctx, &auth.Tenant{
		ID:   "tenant-alpha",
		Name: "Alpha Corp",
		Slug: "alpha",
	})
	if err != nil {
		t.Fatalf("failed to create tenant alpha: %v", err)
	}

	userAlpha, err := authStore.CreateUser(ctx, &auth.User{
		ID:    "usr-alpha-owner",
		Email: "owner@alpha.com",
		Name:  "Alpha Owner",
	})
	if err != nil {
		t.Fatalf("failed to create user alpha: %v", err)
	}

	_, _ = authStore.CreateMembership(ctx, &auth.Membership{
		TenantID: tenantAlpha.ID,
		UserID:   userAlpha.ID,
		Role:     auth.RoleOwner,
	})

	rawTokenAlpha, err := createTestAPIToken(ctx, authStore, tenantAlpha.ID, userAlpha.ID, "Alpha Owner Token", auth.RoleOwner, nil)
	if err != nil {
		t.Fatalf("failed to create token alpha: %v", err)
	}

	// 2. Create Tenant Beta
	tenantBeta, err := authStore.CreateTenant(ctx, &auth.Tenant{
		ID:   "tenant-beta",
		Name: "Beta Corp",
		Slug: "beta",
	})
	if err != nil {
		t.Fatalf("failed to create tenant beta: %v", err)
	}

	userBeta, err := authStore.CreateUser(ctx, &auth.User{
		ID:    "usr-beta-owner",
		Email: "owner@beta.com",
		Name:  "Beta Owner",
	})
	if err != nil {
		t.Fatalf("failed to create user beta: %v", err)
	}

	_, _ = authStore.CreateMembership(ctx, &auth.Membership{
		TenantID: tenantBeta.ID,
		UserID:   userBeta.ID,
		Role:     auth.RoleOwner,
	})

	rawTokenBeta, err := createTestAPIToken(ctx, authStore, tenantBeta.ID, userBeta.ID, "Beta Owner Token", auth.RoleOwner, nil)
	if err != nil {
		t.Fatalf("failed to create token beta: %v", err)
	}

	// 3. Create a Viewer in Tenant Alpha
	userViewer, err := authStore.CreateUser(ctx, &auth.User{
		ID:    "usr-alpha-viewer",
		Email: "viewer@alpha.com",
		Name:  "Alpha Viewer",
	})
	if err != nil {
		t.Fatalf("failed to create viewer user: %v", err)
	}

	_, _ = authStore.CreateMembership(ctx, &auth.Membership{
		TenantID: tenantAlpha.ID,
		UserID:   userViewer.ID,
		Role:     auth.RoleViewer,
	})

	rawTokenViewer, err := createTestAPIToken(ctx, authStore, tenantAlpha.ID, userViewer.ID, "Alpha Viewer Token", auth.RoleViewer, nil)
	if err != nil {
		t.Fatalf("failed to create viewer token: %v", err)
	}

	// 4. Create a Scoped Token (workflows:read ONLY) in Tenant Alpha
	rawTokenScoped, err := createTestAPIToken(ctx, authStore, tenantAlpha.ID, userAlpha.ID, "Alpha Read-Only Scoped Token", auth.RoleViewer, []string{string(auth.PermWorkflowsRead)})
	if err != nil {
		t.Fatalf("failed to create scoped token: %v", err)
	}

	return &multiTenantEnv{
		server:      ts,
		authStore:   authStore,
		wfStore:     wfStore,
		execStore:   execStore,
		dlStore:     dlStore,
		credStore:   credStore,
		whStore:     whStore,
		eventStore:  eventStore,
		tokenAlpha:  rawTokenAlpha,
		tokenBeta:   rawTokenBeta,
		tokenViewer: rawTokenViewer,
		tokenScoped: rawTokenScoped,
	}
}

func doRequest(t *testing.T, method, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(raw)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return resp, buf.Bytes()
}

func TestCrossTenantAccessDenial(t *testing.T) {
	env := setupMultiTenantTestServer(t)
	defer env.server.Close()

	baseURL := env.server.URL

	// 1. Alpha creates a workflow
	wfPayload := map[string]any{
		"name": "Alpha Secret Pipeline",
		"trigger": map[string]any{
			"type": "webhook",
		},
		"actions": []map[string]any{
			{
				"integration": "http",
				"action_type": "request",
				"config": map[string]any{
					"url":    "https://example.com",
					"method": "POST",
				},
			},
		},
	}
	resp, body := doRequest(t, http.MethodPost, baseURL+"/api/v1/workflows", env.tokenAlpha, wfPayload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating Alpha workflow, got %d: %s", resp.StatusCode, string(body))
	}
	var createdWf workflow.Workflow
	_ = json.Unmarshal(body, &createdWf)
	if createdWf.ID == "" || createdWf.TenantID != "tenant-alpha" {
		t.Fatalf("invalid workflow created: %+v", createdWf)
	}

	// 2. Beta attempts to read Alpha's workflow -> 403 Forbidden (CROSS_TENANT_DENIED)
	resp, body = doRequest(t, http.MethodGet, baseURL+"/api/v1/workflows/"+createdWf.ID, env.tokenBeta, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 reading cross-tenant workflow, got %d: %s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "CROSS_TENANT_DENIED") {
		t.Fatalf("expected error code CROSS_TENANT_DENIED, got: %s", string(body))
	}

	// 3. Beta attempts to update Alpha's workflow -> 403 Forbidden (CROSS_TENANT_DENIED)
	updatePayload := map[string]any{
		"name": "Hacked Pipeline Name",
		"trigger": map[string]any{
			"type": "webhook",
		},
		"actions": []map[string]any{
			{
				"integration": "http",
				"action_type": "request",
				"config":      map[string]any{"url": "https://evil.com"},
			},
		},
	}
	resp, body = doRequest(t, http.MethodPut, baseURL+"/api/v1/workflows/"+createdWf.ID, env.tokenBeta, updatePayload)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 updating cross-tenant workflow, got %d: %s", resp.StatusCode, string(body))
	}

	// 4. Beta attempts to delete Alpha's workflow -> 403 Forbidden (CROSS_TENANT_DENIED)
	resp, body = doRequest(t, http.MethodDelete, baseURL+"/api/v1/workflows/"+createdWf.ID, env.tokenBeta, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 deleting cross-tenant workflow, got %d: %s", resp.StatusCode, string(body))
	}

	// 5. Beta lists workflows -> Alpha's workflow must NOT appear in Beta's list
	resp, body = doRequest(t, http.MethodGet, baseURL+"/api/v1/workflows", env.tokenBeta, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 listing workflows for Beta, got %d: %s", resp.StatusCode, string(body))
	}
	var betaList []*workflow.Workflow
	_ = json.Unmarshal(body, &betaList)
	for _, wf := range betaList {
		if wf.ID == createdWf.ID {
			t.Fatalf("isolation failure: Beta saw Alpha workflow %s", wf.ID)
		}
	}

	// 6. Alpha creates a credential
	credPayload := map[string]any{
		"name":        "Alpha API Key",
		"integration": "http",
		"values": map[string]string{
			"token": "alpha-secret-key-12345",
		},
	}
	resp, body = doRequest(t, http.MethodPost, baseURL+"/api/v1/credentials", env.tokenAlpha, credPayload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating Alpha credential, got %d: %s", resp.StatusCode, string(body))
	}
	var createdCred credentials.Credential
	_ = json.Unmarshal(body, &createdCred)

	// 7. Beta attempts to read Alpha's credential -> 403 Forbidden (CROSS_TENANT_DENIED)
	resp, body = doRequest(t, http.MethodGet, baseURL+"/api/v1/credentials/"+createdCred.ID, env.tokenBeta, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 reading cross-tenant credential, got %d: %s", resp.StatusCode, string(body))
	}

	// 8. Beta attempts to delete Alpha's credential -> 403 Forbidden (CROSS_TENANT_DENIED)
	resp, body = doRequest(t, http.MethodDelete, baseURL+"/api/v1/credentials/"+createdCred.ID, env.tokenBeta, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 deleting cross-tenant credential, got %d: %s", resp.StatusCode, string(body))
	}

	// 9. Alpha creates a Webhook
	whPayload := map[string]any{
		"name":                  "Alpha Webhook Endpoint",
		"rate_limit_per_minute": 60,
		"enabled":               true,
	}
	resp, body = doRequest(t, http.MethodPost, baseURL+"/api/v1/webhooks", env.tokenAlpha, whPayload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating Alpha webhook, got %d: %s", resp.StatusCode, string(body))
	}
	var createdWh webhooks.Webhook
	_ = json.Unmarshal(body, &createdWh)

	// 10. Beta attempts to read Alpha's webhook -> 403 Forbidden (CROSS_TENANT_DENIED)
	resp, body = doRequest(t, http.MethodGet, baseURL+"/api/v1/webhooks/"+createdWh.ID, env.tokenBeta, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 reading cross-tenant webhook, got %d: %s", resp.StatusCode, string(body))
	}

	// 11. Alpha creates an execution directly in execStore
	execAlpha, err := env.execStore.Create(context.Background(), &workflow.Execution{
		ID:         "exec-alpha-1",
		TenantID:   "tenant-alpha",
		WorkflowID: createdWf.ID,
		EventID:    "evt-alpha-1",
		Status:     workflow.ExecutionFailed,
	})
	if err != nil {
		t.Fatalf("failed to insert alpha execution: %v", err)
	}

	// 12. Beta attempts to get Alpha's execution -> 403 Forbidden (CROSS_TENANT_DENIED)
	resp, body = doRequest(t, http.MethodGet, baseURL+"/api/v1/executions/"+execAlpha.ID, env.tokenBeta, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 accessing cross-tenant execution, got %d: %s", resp.StatusCode, string(body))
	}

	// 13. Beta attempts to retry Alpha's execution -> 403 Forbidden (CROSS_TENANT_DENIED)
	resp, body = doRequest(t, http.MethodPost, baseURL+"/api/v1/executions/"+execAlpha.ID+"/retry", env.tokenBeta, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 retrying cross-tenant execution, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestRBACPermissionsEnforcement(t *testing.T) {
	env := setupMultiTenantTestServer(t)
	defer env.server.Close()

	baseURL := env.server.URL

	// 1. Viewer lists workflows -> Allowed (200 OK)
	resp, body := doRequest(t, http.MethodGet, baseURL+"/api/v1/workflows", env.tokenViewer, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for viewer listing workflows, got %d: %s", resp.StatusCode, string(body))
	}

	// 2. Viewer attempts to create workflow -> 403 Forbidden (PERMISSION_DENIED)
	wfPayload := map[string]any{
		"name": "Viewer Forbidden Workflow",
		"trigger": map[string]any{
			"type": "webhook",
		},
		"actions": []map[string]any{
			{
				"integration": "http",
				"action_type": "request",
				"config":      map[string]any{"url": "https://example.com"},
			},
		},
	}
	resp, body = doRequest(t, http.MethodPost, baseURL+"/api/v1/workflows", env.tokenViewer, wfPayload)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer creating workflow, got %d: %s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "PERMISSION_DENIED") {
		t.Fatalf("expected PERMISSION_DENIED error code, got: %s", string(body))
	}

	// 3. Viewer attempts to invite member -> 403 Forbidden (PERMISSION_DENIED)
	memberPayload := map[string]any{
		"email": "newuser@alpha.com",
		"role":  "viewer",
	}
	resp, body = doRequest(t, http.MethodPost, baseURL+"/api/v1/tenants/current/members", env.tokenViewer, memberPayload)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer inviting member, got %d: %s", resp.StatusCode, string(body))
	}

	// 4. Viewer attempts to create API token -> 403 Forbidden (PERMISSION_DENIED)
	tokenPayload := map[string]any{
		"name":   "Viewer Token Attempt",
		"scopes": []string{"workflows:read"},
	}
	resp, body = doRequest(t, http.MethodPost, baseURL+"/api/v1/tokens", env.tokenViewer, tokenPayload)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer creating token, got %d: %s", resp.StatusCode, string(body))
	}

	// 5. Owner creates API token -> Allowed (201 Created)
	tokenOwnerPayload := map[string]any{
		"name":   "New Service Account",
		"role":   "operator",
		"scopes": []string{"workflows:read", "workflows:write"},
	}
	resp, body = doRequest(t, http.MethodPost, baseURL+"/api/v1/tokens", env.tokenAlpha, tokenOwnerPayload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for owner creating token, got %d: %s", resp.StatusCode, string(body))
	}
	var createdToken struct {
		ID       string `json:"id"`
		RawToken string `json:"raw_token"`
	}
	_ = json.Unmarshal(body, &createdToken)
	if createdToken.RawToken == "" || !strings.HasPrefix(createdToken.RawToken, "krb_") {
		t.Fatalf("expected plaintext token prefixed with krb_, got: %+v", createdToken)
	}

	// 6. Owner revokes the token -> Allowed (200 OK or 204 No Content)
	resp, body = doRequest(t, http.MethodDelete, baseURL+"/api/v1/tokens/"+createdToken.ID, env.tokenAlpha, nil)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 200 or 204 revoking token, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestScopedAPITokens(t *testing.T) {
	env := setupMultiTenantTestServer(t)
	defer env.server.Close()

	baseURL := env.server.URL

	// 1. Scoped token (workflows:read) gets workflows -> 200 OK
	resp, body := doRequest(t, http.MethodGet, baseURL+"/api/v1/workflows", env.tokenScoped, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 reading workflows with scoped token, got %d: %s", resp.StatusCode, string(body))
	}

	// 2. Scoped token (workflows:read) attempts to write workflow -> 403 Forbidden (PERMISSION_DENIED)
	wfPayload := map[string]any{
		"name": "Scoped Token Forbidden Workflow",
		"trigger": map[string]any{
			"type": "webhook",
		},
		"actions": []map[string]any{
			{
				"integration": "http",
				"action_type": "request",
				"config":      map[string]any{"url": "https://example.com"},
			},
		},
	}
	resp, body = doRequest(t, http.MethodPost, baseURL+"/api/v1/workflows", env.tokenScoped, wfPayload)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 writing workflow with read-only scoped token, got %d: %s", resp.StatusCode, string(body))
	}

	// 3. Scoped token attempts to access credentials -> 403 Forbidden (PERMISSION_DENIED)
	resp, body = doRequest(t, http.MethodGet, baseURL+"/api/v1/credentials", env.tokenScoped, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 reading credentials with workflows:read scoped token, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestAuditLoggingAndSecretScrubbing(t *testing.T) {
	env := setupMultiTenantTestServer(t)
	defer env.server.Close()

	baseURL := env.server.URL

	// 1. Create a credential with sensitive values
	rawSecret := "super-secret-api-token-value-xyz"
	credPayload := map[string]any{
		"name":        "Stripe Live API Key",
		"integration": "http",
		"values": map[string]string{
			"secret_token": rawSecret,
			"api_key":      "sk_live_1234567890abcdef",
		},
	}
	resp, body := doRequest(t, http.MethodPost, baseURL+"/api/v1/credentials", env.tokenAlpha, credPayload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating credential, got %d: %s", resp.StatusCode, string(body))
	}
	var createdCred credentials.Credential
	_ = json.Unmarshal(body, &createdCred)

	// 2. Read the credential by ID
	resp, body = doRequest(t, http.MethodGet, baseURL+"/api/v1/credentials/"+createdCred.ID, env.tokenAlpha, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 reading credential, got %d: %s", resp.StatusCode, string(body))
	}

	// 3. Inspect audit logs for Tenant Alpha
	resp, body = doRequest(t, http.MethodGet, baseURL+"/api/v1/audit-logs", env.tokenAlpha, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 getting audit logs, got %d: %s", resp.StatusCode, string(body))
	}
	var logs []*auth.AuditLog
	_ = json.Unmarshal(body, &logs)

	if len(logs) == 0 {
		t.Fatalf("expected audit logs to be recorded, found none")
	}

	// 4. Verify no secret tokens exist in any audit log details
	auditDump, _ := json.Marshal(logs)
	dumpStr := string(auditDump)
	if strings.Contains(dumpStr, rawSecret) || strings.Contains(dumpStr, "sk_live_1234567890abcdef") {
		t.Fatalf("SECURITY VIOLATION: Plaintext secret leaked into audit logs: %s", dumpStr)
	}

	// 5. Verify credential.created and credential.read events exist
	var hasCreated, hasRead bool
	for _, l := range logs {
		if l.Action == "credential.created" && l.ResourceID == createdCred.ID {
			hasCreated = true
		}
		if l.Action == "credential.read" && l.ResourceID == createdCred.ID {
			hasRead = true
		}
	}
	if !hasCreated || !hasRead {
		t.Fatalf("expected both credential.created and credential.read in audit trail, got: %+v", logs)
	}
}

func TestSingleTenantDevFallback(t *testing.T) {
	// Create server with MultiTenancyEnabled = false (dev mode)
	authStore := auth.NewMemoryStore()
	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore([]byte("01234567890123456789012345678901"))
	whStore := webhooks.NewMemoryStore()

	handlers := api.NewHandlersFromConfig(api.Config{
		Workflows:    wfStore,
		Executions:   execStore,
		DeadLetters:  dlStore,
		Creds:        credStore,
		WebhookStore: whStore,
		AuthStore:    authStore,
		MultiTenancy: false, // Dev single-tenant mode
	})

	server := api.NewServerWithOptions("0", handlers, api.ServerOptions{
		AuthStore:           authStore,
		MultiTenancyEnabled: false,
		DefaultTenantID:     "default",
	})
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Calling GET /api/v1/auth/me without an Authorization header should return default owner identity
	resp, body := doRequest(t, http.MethodGet, ts.URL+"/api/v1/auth/me", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for dev /auth/me fallback, got %d: %s", resp.StatusCode, string(body))
	}
	var me map[string]any
	_ = json.Unmarshal(body, &me)
	if me["tenant_id"] != "default" || me["role"] != "owner" {
		t.Fatalf("expected default owner identity, got: %+v", me)
	}

	// Creating a workflow in single-tenant dev mode without token succeeds
	wfPayload := map[string]any{
		"name": "Single Tenant Dev Pipeline",
		"trigger": map[string]any{
			"type": "webhook",
		},
		"actions": []map[string]any{
			{
				"integration": "http",
				"action_type": "request",
				"config":      map[string]any{"url": "https://example.com"},
			},
		},
	}
	resp, body = doRequest(t, http.MethodPost, ts.URL+"/api/v1/workflows", "", wfPayload)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating workflow in single-tenant dev mode, got %d: %s", resp.StatusCode, string(body))
	}
}
