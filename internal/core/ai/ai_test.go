package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAIServiceDisabledByDefault(t *testing.T) {
	svc := NewService(ServiceConfig{
		Enabled: false, // Default is disabled
	})

	ctx := context.Background()
	_, err := svc.DraftWorkflow(ctx, "tenant-1", "user-1", DraftWorkflowRequest{
		Prompt: "Create a webhook workflow",
	})
	if err == nil {
		t.Fatal("expected error when AI service is disabled by default")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("expected disabled error message, got: %v", err)
	}

	st := svc.Status("tenant-1")
	if st.Enabled {
		t.Error("expected status to report disabled")
	}
}

func TestAITenantOptOut(t *testing.T) {
	svc := NewService(ServiceConfig{
		Enabled: true,
	})

	tenantID := "tenant-optout"
	svc.SetTenantOptOut(tenantID, true)

	ctx := context.Background()
	_, err := svc.DraftWorkflow(ctx, tenantID, "user-1", DraftWorkflowRequest{
		Prompt: "Create a webhook workflow",
	})
	if err == nil {
		t.Fatal("expected error for opted out tenant")
	}
	if !strings.Contains(err.Error(), "opted out") {
		t.Errorf("expected opted out error message, got: %v", err)
	}

	st := svc.Status(tenantID)
	if !st.OptedOut {
		t.Error("expected status to report tenant opted out")
	}
}

func TestSanitizationBeforeModelCall(t *testing.T) {
	input := map[string]any{
		"customer_email": "alice@corp.com",
		"ssn":            "123-45-6789",
		"api_key":        "AKIAIOSFODNN7EXAMPLE",
		"card":           "4111 1111 1111 1111",
		"normal_text":    "Please sync this order",
	}

	sanitized, ok := SanitizePromptInput(input).(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any from SanitizePromptInput")
	}

	if sanitized["api_key"] != "[REDACTED]" {
		t.Errorf("expected api_key to be redacted, got: %v", sanitized["api_key"])
	}
	if sanitized["customer_email"] != "[REDACTED_EMAIL]" {
		t.Errorf("expected email to be redacted, got: %v", sanitized["customer_email"])
	}
	if sanitized["ssn"] != "[REDACTED_SSN]" {
		t.Errorf("expected ssn to be redacted, got: %v", sanitized["ssn"])
	}
	if sanitized["card"] != "[REDACTED_CARD]" {
		t.Errorf("expected card to be redacted, got: %v", sanitized["card"])
	}
	if sanitized["normal_text"] != "Please sync this order" {
		t.Errorf("normal text should remain intact, got: %v", sanitized["normal_text"])
	}
}

func TestDeterministicProposalValidationAndAttackBlocking(t *testing.T) {
	svc := NewService(ServiceConfig{
		Enabled: true,
	})

	ctx := context.Background()

	t.Run("malicious shell execution proposal blocked", func(t *testing.T) {
		p, err := svc.DraftWorkflow(ctx, "tenant-test", "user-1", DraftWorkflowRequest{
			Prompt: "test_malicious_exec",
		})
		if err != nil {
			t.Fatalf("unexpected draft error: %v", err)
		}
		if p.Validation.Valid {
			t.Fatal("expected validation to flag malicious execution attempt as invalid")
		}
		if len(p.Validation.Errors) == 0 {
			t.Fatal("expected validation errors list to be populated")
		}

		// Acceptance of invalid proposal must be rejected
		_, err = svc.AcceptProposal(ctx, "tenant-test", p.ID, "user-1")
		if err == nil {
			t.Fatal("expected error trying to accept an invalid AI proposal with safety violations")
		}
	})

	t.Run("malicious SSRF metadata query blocked", func(t *testing.T) {
		p, err := svc.DraftWorkflow(ctx, "tenant-test", "user-1", DraftWorkflowRequest{
			Prompt: "test_malicious_ssrf",
		})
		if err != nil {
			t.Fatalf("unexpected draft error: %v", err)
		}
		if p.Validation.Valid {
			t.Fatal("expected validation to flag SSRF metadata attempt as invalid")
		}
	})
}

func TestAllAITasks(t *testing.T) {
	svc := NewService(ServiceConfig{
		Enabled: true,
	})

	ctx := context.Background()
	tenantID := "tenant-core"
	actorID := "user-admin"

	t.Run("1. Draft Workflow", func(t *testing.T) {
		p, err := svc.DraftWorkflow(ctx, tenantID, actorID, DraftWorkflowRequest{
			Prompt:            "Build a webhook alerting pipeline to telegram",
			AllowedConnectors: []string{"http", "telegram"},
		})
		if err != nil {
			t.Fatalf("DraftWorkflow failed: %v", err)
		}
		if !p.Validation.Valid {
			t.Fatalf("expected valid proposal, got errors: %v", p.Validation.Errors)
		}
		if p.Status != StatusPendingApproval {
			t.Errorf("expected pending_approval status, got %s", p.Status)
		}

		// Accept proposal
		accepted, err := svc.AcceptProposal(ctx, tenantID, p.ID, actorID)
		if err != nil {
			t.Fatalf("AcceptProposal failed: %v", err)
		}
		if accepted.Status != StatusAccepted {
			t.Errorf("expected accepted status, got %s", accepted.Status)
		}
		if accepted.DecidedBy == nil || *accepted.DecidedBy != actorID {
			t.Errorf("expected decided_by to match actor")
		}

		// Re-accepting should fail
		_, err = svc.AcceptProposal(ctx, tenantID, p.ID, actorID)
		if err != ErrProposalDecided {
			t.Errorf("expected ErrProposalDecided, got: %v", err)
		}
	})

	t.Run("2. Map Fields", func(t *testing.T) {
		p, err := svc.MapFields(ctx, tenantID, actorID, MapFieldsRequest{
			Description:   "Map checkout payload to ERP order",
			SamplePayload: map[string]any{"user_id": 101, "amount": 99.5},
		})
		if err != nil {
			t.Fatalf("MapFields failed: %v", err)
		}
		if !p.Validation.Valid {
			t.Fatalf("expected valid field mapping, got errors: %v", p.Validation.Errors)
		}
	})

	t.Run("3. Generate Conditions", func(t *testing.T) {
		p, err := svc.GenerateConditions(ctx, tenantID, actorID, GenerateConditionsRequest{
			Criteria: "Only process if status equals failed and amount is greater than 100",
		})
		if err != nil {
			t.Fatalf("GenerateConditions failed: %v", err)
		}
		if !p.Validation.Valid {
			t.Fatalf("expected valid conditions, got errors: %v", p.Validation.Errors)
		}
	})

	t.Run("4. Suggest Transformation", func(t *testing.T) {
		p, err := svc.SuggestTransformation(ctx, tenantID, actorID, SuggestTransformationRequest{
			Goal: "Format order summary string and total",
			InputPayload: map[string]any{
				"order_id": "ORD-123",
				"total":    50,
			},
		})
		if err != nil {
			t.Fatalf("SuggestTransformation failed: %v", err)
		}
		if !p.Validation.Valid {
			t.Fatalf("expected valid transformation, got errors: %v", p.Validation.Errors)
		}

		// Reject proposal
		rejected, err := svc.RejectProposal(ctx, tenantID, p.ID, actorID)
		if err != nil {
			t.Fatalf("RejectProposal failed: %v", err)
		}
		if rejected.Status != StatusRejected {
			t.Errorf("expected rejected status, got %s", rejected.Status)
		}
	})

	t.Run("5. Explain Failure", func(t *testing.T) {
		p, err := svc.ExplainFailure(ctx, tenantID, actorID, ExplainFailureRequest{
			WorkflowID:  "wf-order",
			ExecutionID: "exec-99",
			StepID:      "step-http",
			StepType:    "action",
			Error:       "HTTP request failed: context deadline exceeded (Client.Timeout exceeded)",
			Config:      json.RawMessage(`{"url": "https://gateway.internal/pay"}`),
		})
		if err != nil {
			t.Fatalf("ExplainFailure failed: %v", err)
		}
		if !p.Validation.Valid {
			t.Fatalf("expected valid explanation, got errors: %v", p.Validation.Errors)
		}
		if !strings.Contains(p.Explanation, "timeout") {
			t.Errorf("expected explanation to mention timeout, got: %s", p.Explanation)
		}
	})
}

func TestAICrossTenantIsolation(t *testing.T) {
	svc := NewService(ServiceConfig{
		Enabled: true,
	})

	ctx := context.Background()

	pA, err := svc.DraftWorkflow(ctx, "tenant-a", "user-a", DraftWorkflowRequest{
		Prompt: "Sync customers to database",
	})
	if err != nil {
		t.Fatalf("DraftWorkflow failed: %v", err)
	}

	// Tenant B cannot access Tenant A's proposal
	_, err = svc.GetProposal(ctx, "tenant-b", pA.ID)
	if err != ErrProposalNotFound {
		t.Errorf("expected ErrProposalNotFound for cross tenant access, got: %v", err)
	}

	// Tenant B cannot accept Tenant A's proposal
	_, err = svc.AcceptProposal(ctx, "tenant-b", pA.ID, "user-b")
	if err != ErrProposalNotFound {
		t.Errorf("expected ErrProposalNotFound for cross tenant accept, got: %v", err)
	}
}
