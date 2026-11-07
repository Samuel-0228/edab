package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `{"status":"ok","workflow":{"name":"test"}}`,
					},
				},
			},
			"usage": map[string]any{
				"total_tokens": 42,
			},
		})
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL, "test-key", "gpt-4o", 5*time.Second)
	if provider.Name() != "openai" {
		t.Errorf("expected provider name openai, got %s", provider.Name())
	}

	ctx := context.Background()
	res, err := provider.Generate(ctx, TaskDraftWorkflow, "system instructions", "user prompt")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if res.TokensUsed != 42 {
		t.Errorf("expected 42 tokens, got %d", res.TokensUsed)
	}
	if len(res.Content) == 0 {
		t.Error("expected non-empty content")
	}

	// Unauthorized
	badProvider := NewOpenAIProvider(server.URL, "bad-key", "gpt-4o", 5*time.Second)
	_, err = badProvider.Generate(ctx, TaskDraftWorkflow, "sys", "usr")
	if err == nil {
		t.Error("expected error for unauthorized provider")
	}
}

func TestServiceAndMemoryStoreProposals(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(ServiceConfig{
		Enabled:  true,
		Store:    store,
		Provider: NewMockProvider(),
	})

	svc.SetEnabled(false)
	svc.SetEnabled(true)

	if svc.Store() != store {
		t.Error("expected svc.Store() to match store")
	}

	// Create and list proposals
	prop, err := store.CreateProposal(ctx, &Proposal{
		TenantID: "tenant-x",
		Task:     TaskDraftWorkflow,
		Status:   StatusPendingApproval,
		Prompt:   "Create pipeline",
	})
	if err != nil {
		t.Fatalf("CreateProposal failed: %v", err)
	}

	list, err := store.ListProposals(ctx, "tenant-x", 10, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListProposals store failed: %v, len=%d", err, len(list))
	}

	svcList, err := svc.ListProposals(ctx, "tenant-x", 10, 0)
	if err != nil || len(svcList) != 1 {
		t.Fatalf("ListProposals svc failed: %v, len=%d", err, len(svcList))
	}

	got, err := store.GetProposal(ctx, "tenant-x", prop.ID)
	if err != nil || got.ID != prop.ID {
		t.Fatalf("GetProposal failed: %v", err)
	}
}
