package governance

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"kerob/internal/core/auth"
	"kerob/internal/core/workflow"
)

func TestMasking(t *testing.T) {
	t.Run("PII masking", func(t *testing.T) {
		input := "User alice@example.com with phone 555-123-4567 and SSN 123-45-6789 registered."
		masked := MaskString(input, true, false)

		if strings.Contains(masked, "alice@example.com") {
			t.Errorf("expected email to be masked, got: %s", masked)
		}
		if !strings.Contains(masked, "[REDACTED_EMAIL]") {
			t.Errorf("expected [REDACTED_EMAIL], got: %s", masked)
		}
		if strings.Contains(masked, "123-45-6789") {
			t.Errorf("expected SSN to be masked, got: %s", masked)
		}
		if !strings.Contains(masked, "[REDACTED_SSN]") {
			t.Errorf("expected [REDACTED_SSN], got: %s", masked)
		}
		if strings.Contains(masked, "555-123-4567") {
			t.Errorf("expected phone to be masked, got: %s", masked)
		}
		if !strings.Contains(masked, "[REDACTED_PHONE]") {
			t.Errorf("expected [REDACTED_PHONE], got: %s", masked)
		}
	})

	t.Run("Credit card with Luhn verification", func(t *testing.T) {
		// Valid test Visa card
		validVisa := "4111 1111 1111 1111"
		maskedValid := MaskString("Paid with "+validVisa, true, false)
		if strings.Contains(maskedValid, "4111") {
			t.Errorf("expected valid card to be masked, got: %s", maskedValid)
		}
		if !strings.Contains(maskedValid, "[REDACTED_CARD]") {
			t.Errorf("expected [REDACTED_CARD], got: %s", maskedValid)
		}

		// Invalid card number (fails Luhn)
		invalidCard := "4111 1111 1111 1112"
		maskedInvalid := MaskString("Order ref "+invalidCard, true, false)
		if strings.Contains(maskedInvalid, "[REDACTED_CARD]") {
			t.Errorf("expected invalid card not to be masked as card, got: %s", maskedInvalid)
		}
	})

	t.Run("Secret tokens masking", func(t *testing.T) {
		input := "AWS key AKIAIOSFODNN7EXAMPLE, GitHub token ghp_1234567890abcdefghijklmnopqrstuvwxyz, and Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozS6-mG_XvT"
		masked := MaskString(input, false, true)

		if strings.Contains(masked, "AKIAIOSFODNN7EXAMPLE") {
			t.Errorf("expected AWS key to be masked")
		}
		if !strings.Contains(masked, "[REDACTED_AWS_KEY]") {
			t.Errorf("expected [REDACTED_AWS_KEY], got: %s", masked)
		}
		if strings.Contains(masked, "ghp_1234567890") {
			t.Errorf("expected GitHub token to be masked")
		}
		if !strings.Contains(masked, "[REDACTED_GITHUB_TOKEN]") {
			t.Errorf("expected [REDACTED_GITHUB_TOKEN], got: %s", masked)
		}
		if strings.Contains(masked, "eyJhbGciOi") {
			t.Errorf("expected JWT to be masked")
		}
	})

	t.Run("Deep map and slice masking", func(t *testing.T) {
		data := map[string]any{
			"api_key": "supersecretkey123",
			"user": map[string]any{
				"email": "bob@acme.corp",
				"notes": "Secret token is ghp_abcdefghijklmnopqrstuvwxyz1234567890",
			},
			"tags": []any{
				"normal",
				"Call 800-555-0199 for help",
			},
		}

		masked := MaskValue(data, true, true).(map[string]any)

		if masked["api_key"] != "[REDACTED]" {
			t.Errorf("expected api_key to be [REDACTED], got: %v", masked["api_key"])
		}

		userMap := masked["user"].(map[string]any)
		if userMap["email"] != "[REDACTED_EMAIL]" {
			t.Errorf("expected email to be [REDACTED_EMAIL], got: %v", userMap["email"])
		}
		if strings.Contains(userMap["notes"].(string), "ghp_") {
			t.Errorf("expected github token in notes to be masked")
		}

		tags := masked["tags"].([]any)
		if strings.Contains(tags[1].(string), "800-555-0199") {
			t.Errorf("expected phone in tags to be masked")
		}
	})
}

func TestAuditCryptographicChain(t *testing.T) {
	tenantID := "tenant-alpha"
	now := time.Now().UTC()

	// 1. Generate chain of 3 audit events
	log1 := &auth.AuditLog{
		ID:           "log-1",
		TenantID:     tenantID,
		SequenceNum:  1,
		PrevHash:     GenesisHash,
		ActorID:      "user-1",
		ActorType:    "user",
		Action:       "workflow.create",
		ResourceType: "workflow",
		ResourceID:   "wf-1",
		Details:      map[string]any{"name": "Order Pipeline"},
		CreatedAt:    now.Add(1 * time.Second),
	}
	log1.EntryHash = ComputeAuditEntryHash(log1)

	log2 := &auth.AuditLog{
		ID:           "log-2",
		TenantID:     tenantID,
		SequenceNum:  2,
		PrevHash:     log1.EntryHash,
		ActorID:      "user-1",
		ActorType:    "user",
		Action:       "workflow.update",
		ResourceType: "workflow",
		ResourceID:   "wf-1",
		Details:      map[string]any{"name": "Order Pipeline V2"},
		CreatedAt:    now.Add(2 * time.Second),
	}
	log2.EntryHash = ComputeAuditEntryHash(log2)

	log3 := &auth.AuditLog{
		ID:           "log-3",
		TenantID:     tenantID,
		SequenceNum:  3,
		PrevHash:     log2.EntryHash,
		ActorID:      "user-2",
		ActorType:    "user",
		Action:       "workflow.delete",
		ResourceType: "workflow",
		ResourceID:   "wf-1",
		Details:      map[string]any{"reason": "cleanup"},
		CreatedAt:    now.Add(3 * time.Second),
	}
	log3.EntryHash = ComputeAuditEntryHash(log3)

	chain := []*auth.AuditLog{log1, log2, log3}

	// Verify valid chain
	res := VerifyAuditChain(tenantID, chain)
	if !res.Valid {
		t.Fatalf("expected audit chain to be valid, got error: %s", res.ErrorMessage)
	}
	if res.TotalRecords != 3 {
		t.Errorf("expected 3 verified records, got %d", res.TotalRecords)
	}

	// Test Tampering Detection
	t.Run("detect modified details", func(t *testing.T) {
		tamperedLog2 := *log2
		tamperedLog2.Action = "malicious.mutation"
		tamperedChain := []*auth.AuditLog{log1, &tamperedLog2, log3}

		tamperedRes := VerifyAuditChain(tenantID, tamperedChain)
		if tamperedRes.Valid {
			t.Fatal("expected tampering detection to fail valid check")
		}
		if tamperedRes.BrokenAtSequence == nil || *tamperedRes.BrokenAtSequence != 2 {
			t.Errorf("expected broken sequence 2, got %v", tamperedRes.BrokenAtSequence)
		}
	})

	t.Run("detect broken sequence gap", func(t *testing.T) {
		brokenChain := []*auth.AuditLog{log1, log3} // log 2 deleted
		brokenRes := VerifyAuditChain(tenantID, brokenChain)
		if brokenRes.Valid {
			t.Fatal("expected broken chain check to fail")
		}
		if brokenRes.BrokenAtSequence == nil || *brokenRes.BrokenAtSequence != 3 {
			t.Errorf("expected broken sequence at 3, got %v", brokenRes.BrokenAtSequence)
		}
	})

	t.Run("detect cross tenant entry injection", func(t *testing.T) {
		injected := *log2
		injected.TenantID = "tenant-evil"
		crossChain := []*auth.AuditLog{log1, &injected, log3}

		crossRes := VerifyAuditChain(tenantID, crossChain)
		if crossRes.Valid {
			t.Fatal("expected cross-tenant injection to be detected")
		}
	})

	t.Run("export JSON and CSV", func(t *testing.T) {
		var jsonBuf bytes.Buffer
		if err := ExportAuditLogsJSON(&jsonBuf, chain); err != nil {
			t.Fatalf("export json failed: %v", err)
		}
		if !strings.Contains(jsonBuf.String(), "workflow.create") {
			t.Errorf("expected json export to contain action")
		}

		var csvBuf bytes.Buffer
		if err := ExportAuditLogsCSV(&csvBuf, chain); err != nil {
			t.Fatalf("export csv failed: %v", err)
		}
		csvContent := csvBuf.String()
		if !strings.Contains(csvContent, "sequence_num") || !strings.Contains(csvContent, "tenant-alpha") {
			t.Errorf("expected csv export to contain headers and tenant, got: %s", csvContent)
		}
	})
}

func TestGovernancePublishValidation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	engine := NewGovernanceEngine(store)

	tenantID := "tenant-corp"
	policy := &Policy{
		TenantID:                              tenantID,
		AllowedConnectors:                     []string{"http", "telegram"},
		AllowedOutboundDomains:                []string{"api.telegram.org", "*.acme.internal"},
		MaxWorkflowRetries:                    3,
		MaxPayloadSizeBytes:                   500000,
		DataRetentionDays:                     60,
		RequireApprovalForIrreversibleActions: true,
		BlockedKeywords:                       []string{"malware", "drop database"},
		EnforcementMode:                       ModeEnforce,
	}
	_, err := store.UpsertPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("upsert policy failed: %v", err)
	}

	t.Run("valid workflow passes", func(t *testing.T) {
		wf := &workflow.Workflow{
			ID:         "wf-ok",
			TenantID:   tenantID,
			Name:       "Sync Contacts",
			MaxRetries: 2,
			Actions: []workflow.ActionStep{
				{
					ID:          "step-1",
					Integration: "http",
					Config:      json.RawMessage(`{"url": "https://api.acme.internal/v1/sync"}`),
				},
			},
		}

		violations := engine.ValidateWorkflowPublish(ctx, tenantID, wf)
		if len(violations) > 0 {
			t.Fatalf("expected 0 violations, got: %v", violations)
		}
	})

	t.Run("unallowed connector rejected", func(t *testing.T) {
		wf := &workflow.Workflow{
			ID:         "wf-unallowed-conn",
			TenantID:   tenantID,
			Name:       "Discord Notifier",
			MaxRetries: 2,
			Actions: []workflow.ActionStep{
				{
					ID:          "step-1",
					Integration: "discord", // not in [http, telegram]
				},
			},
		}

		violations := engine.ValidateWorkflowPublish(ctx, tenantID, wf)
		if len(violations) == 0 {
			t.Fatal("expected violation for unallowed connector")
		}
		if violations[0].Rule != "allowed_connectors" {
			t.Errorf("expected rule allowed_connectors, got %s", violations[0].Rule)
		}
	})

	t.Run("outbound domain blocked", func(t *testing.T) {
		wf := &workflow.Workflow{
			ID:         "wf-blocked-domain",
			TenantID:   tenantID,
			Name:       "Exfiltration",
			MaxRetries: 1,
			Actions: []workflow.ActionStep{
				{
					ID:          "step-1",
					Integration: "http",
					Config:      json.RawMessage(`{"url": "https://attacker.com/leak"}`),
				},
			},
		}

		violations := engine.ValidateWorkflowPublish(ctx, tenantID, wf)
		if len(violations) == 0 {
			t.Fatal("expected violation for blocked outbound domain")
		}
		if violations[0].Rule != "outbound_domains" {
			t.Errorf("expected rule outbound_domains, got %s", violations[0].Rule)
		}
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		wf := &workflow.Workflow{
			ID:         "wf-high-retries",
			TenantID:   tenantID,
			Name:       "Aggressive Retrier",
			MaxRetries: 10, // policy limit is 3
			Actions: []workflow.ActionStep{
				{
					ID:          "step-1",
					Integration: "http",
					Config:      json.RawMessage(`{"url": "https://api.telegram.org/bot"}`),
				},
			},
		}

		violations := engine.ValidateWorkflowPublish(ctx, tenantID, wf)
		if len(violations) == 0 {
			t.Fatal("expected violation for exceeded max retries")
		}
		if violations[0].Rule != "max_retries" {
			t.Errorf("expected rule max_retries, got %s", violations[0].Rule)
		}
	})

	t.Run("irreversible action requires approval", func(t *testing.T) {
		// Destructive action without approval
		wf := &workflow.Workflow{
			ID:         "wf-destructive",
			TenantID:   tenantID,
			Name:       "Purge Account",
			MaxRetries: 1,
			Actions: []workflow.ActionStep{
				{
					ID:          "step-1",
					Integration: "http",
					ActionType:  "delete_customer_records",
					Config:      json.RawMessage(`{"url": "https://api.acme.internal/delete"}`),
				},
			},
		}

		violations := engine.ValidateWorkflowPublish(ctx, tenantID, wf)
		if len(violations) == 0 {
			t.Fatal("expected violation for irreversible action without approval")
		}
		if violations[0].Rule != "approval_required" {
			t.Errorf("expected rule approval_required, got %s", violations[0].Rule)
		}

		// When step has explicit approval config, it passes
		wf.Actions[0].Approval = &workflow.ApprovalConfig{
			Title:         "Approve Account Deletion",
			ApproverRoles: []string{"admin"},
		}
		violationsAfterApproval := engine.ValidateWorkflowPublish(ctx, tenantID, wf)
		if len(violationsAfterApproval) > 0 {
			t.Errorf("expected 0 violations when approval attached, got: %v", violationsAfterApproval)
		}
	})

	t.Run("blocked keyword in workflow definition", func(t *testing.T) {
		wf := &workflow.Workflow{
			ID:         "wf-keyword",
			TenantID:   tenantID,
			Name:       "Deploy Malware Service", // contains "malware"
			MaxRetries: 1,
			Actions: []workflow.ActionStep{
				{
					ID:          "step-1",
					Integration: "http",
					Config:      json.RawMessage(`{"url": "https://api.telegram.org/bot"}`),
				},
			},
		}

		violations := engine.ValidateWorkflowPublish(ctx, tenantID, wf)
		if len(violations) == 0 {
			t.Fatal("expected violation for blocked keyword")
		}
		if violations[0].Rule != "blocked_keywords" {
			t.Errorf("expected rule blocked_keywords, got %s", violations[0].Rule)
		}
	})
}

func TestGovernanceRuntimeEnforcement(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	engine := NewGovernanceEngine(store)

	tenantID := "tenant-beta"
	policy := &Policy{
		TenantID:               tenantID,
		AllowedConnectors:      []string{"http"},
		AllowedOutboundDomains: []string{"*.stripe.com"},
		MaxPayloadSizeBytes:    100, // strictly 100 bytes limit
		EnforcementMode:        ModeEnforce,
	}
	_, err := store.UpsertPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("upsert policy failed: %v", err)
	}

	t.Run("payload size exceeded blocks runtime", func(t *testing.T) {
		act := workflow.ActionStep{
			ID:          "act-1",
			Integration: "http",
		}
		hugePayload := map[string]any{
			"blob": strings.Repeat("A", 200),
		}

		err := engine.EnforceActionRuntime(ctx, tenantID, act, hugePayload, json.RawMessage(`{"url": "https://api.stripe.com/v1"}`))
		if err == nil {
			t.Fatal("expected runtime enforcement error due to payload size limit")
		}
		pErr, ok := err.(*PolicyViolationError)
		if !ok {
			t.Fatalf("expected PolicyViolationError, got: %T", err)
		}
		if len(pErr.Violations) == 0 || pErr.Violations[0].Rule != "payload_size" {
			t.Errorf("expected payload_size violation, got: %v", pErr.Violations)
		}
	})

	t.Run("outbound domain disallowed blocks runtime", func(t *testing.T) {
		act := workflow.ActionStep{
			ID:          "act-1",
			Integration: "http",
		}
		smallPayload := map[string]any{"ok": true}

		err := engine.EnforceActionRuntime(ctx, tenantID, act, smallPayload, json.RawMessage(`{"url": "https://evil.org/collect"}`))
		if err == nil {
			t.Fatal("expected runtime enforcement error due to prohibited outbound domain")
		}
	})

	t.Run("audit mode allows execution without blocking", func(t *testing.T) {
		auditPolicy := *policy
		auditPolicy.EnforcementMode = ModeAudit
		_, _ = store.UpsertPolicy(ctx, &auditPolicy)

		act := workflow.ActionStep{
			ID:          "act-1",
			Integration: "http",
		}
		hugePayload := map[string]any{
			"blob": strings.Repeat("A", 200),
		}

		err := engine.EnforceActionRuntime(ctx, tenantID, act, hugePayload, json.RawMessage(`{"url": "https://api.stripe.com/v1"}`))
		if err != nil {
			t.Errorf("expected audit mode to not return error, got: %v", err)
		}
	})
}

func TestCrossTenantIsolation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	p1 := &Policy{
		TenantID:          "tenant-1",
		AllowedConnectors: []string{"slack"},
	}
	p2 := &Policy{
		TenantID:          "tenant-2",
		AllowedConnectors: []string{"teams"},
	}

	_, _ = store.UpsertPolicy(ctx, p1)
	_, _ = store.UpsertPolicy(ctx, p2)

	res1, _ := store.GetPolicy(ctx, "tenant-1")
	res2, _ := store.GetPolicy(ctx, "tenant-2")

	if len(res1.AllowedConnectors) != 1 || res1.AllowedConnectors[0] != "slack" {
		t.Errorf("tenant 1 policy corrupted: %v", res1.AllowedConnectors)
	}
	if len(res2.AllowedConnectors) != 1 || res2.AllowedConnectors[0] != "teams" {
		t.Errorf("tenant 2 policy corrupted: %v", res2.AllowedConnectors)
	}

	// Deleting tenant-1 does not affect tenant-2
	_ = store.DeletePolicy(ctx, "tenant-1")
	afterDelete1, _ := store.GetPolicy(ctx, "tenant-1")
	afterDelete2, _ := store.GetPolicy(ctx, "tenant-2")

	// Tenant 1 falls back to defaults
	if len(afterDelete1.AllowedConnectors) != 0 {
		t.Errorf("expected tenant 1 to fall back to default empty connectors")
	}
	// Tenant 2 remains intact
	if len(afterDelete2.AllowedConnectors) != 1 || afterDelete2.AllowedConnectors[0] != "teams" {
		t.Errorf("tenant 2 should not be affected by tenant 1 deletion")
	}
}
