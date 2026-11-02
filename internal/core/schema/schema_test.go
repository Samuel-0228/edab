package schema_test

import (
	"context"
	"errors"
	"testing"

	"kerob/internal/core/schema"
)

func TestSchemaRegistrationAndValidation(t *testing.T) {
	reg := schema.NewRegistry(schema.NewMemoryStore())
	ctx := context.Background()

	userSchemaDef := `{
		"type": "object",
		"required": ["user_id", "email"],
		"properties": {
			"user_id": { "type": "string" },
			"email": { "type": "string" },
			"age": { "type": "integer" },
			"role": { "type": "string", "enum": ["admin", "member", "guest"] }
		}
	}`

	// 1. Register version 1
	sc, err := reg.RegisterSchema(ctx, &schema.Schema{
		Name:          "user.created",
		Definition:    userSchemaDef,
		Compatibility: schema.CompatibilityBackward,
	})
	if err != nil {
		t.Fatalf("RegisterSchema failed: %v", err)
	}
	if sc.Version != 1 {
		t.Fatalf("expected version 1, got %d", sc.Version)
	}

	// 2. Validate conforming payload
	validPayload := []byte(`{
		"user_id": "usr_123",
		"email": "alice@example.com",
		"age": 30,
		"role": "admin"
	}`)
	valErrs, err := reg.ValidateEvent(ctx, "", "user.created", validPayload)
	if err != nil {
		t.Fatalf("ValidateEvent error: %v", err)
	}
	if len(valErrs) != 0 {
		t.Fatalf("expected 0 validation errors, got %d: %v", len(valErrs), valErrs)
	}

	// 3. Validate non-conforming payload (missing required field, wrong type, invalid enum)
	invalidPayload := []byte(`{
		"user_id": "usr_456",
		"age": "thirty",
		"role": "superadmin"
	}`)
	valErrs, err = reg.ValidateEvent(ctx, "", "user.created", invalidPayload)
	if err != nil {
		t.Fatalf("ValidateEvent error: %v", err)
	}
	if len(valErrs) != 3 {
		t.Fatalf("expected 3 validation errors, got %d: %v", len(valErrs), valErrs)
	}

	// Verify specific error rules
	hasRequiredErr := false
	hasTypeErr := false
	hasEnumErr := false
	for _, ve := range valErrs {
		if ve.Field == "email" && ve.Rule == "required" {
			hasRequiredErr = true
		}
		if ve.Field == "age" && ve.Rule == "type" {
			hasTypeErr = true
		}
		if ve.Field == "role" && ve.Rule == "enum" {
			hasEnumErr = true
		}
	}
	if !hasRequiredErr || !hasTypeErr || !hasEnumErr {
		t.Fatalf("expected required, type, and enum errors, got: %v", valErrs)
	}

	// 4. Validate unregistered event type allows passthrough
	valErrs, err = reg.ValidateEvent(ctx, "", "unknown.unregistered.event", []byte(`{"anything": 123}`))
	if err != nil {
		t.Fatalf("expected no error for unregistered event, got: %v", err)
	}
	if len(valErrs) != 0 {
		t.Fatalf("expected 0 errors for unregistered event, got: %v", valErrs)
	}
}

func TestSchemaEvolutionCompatibility(t *testing.T) {
	reg := schema.NewRegistry(schema.NewMemoryStore())
	ctx := context.Background()

	v1Def := `{
		"type": "object",
		"required": ["id"],
		"properties": {
			"id": { "type": "string" },
			"name": { "type": "string" }
		}
	}`

	_, err := reg.RegisterSchema(ctx, &schema.Schema{
		Name:          "customer.updated",
		Definition:    v1Def,
		Compatibility: schema.CompatibilityBackward,
	})
	if err != nil {
		t.Fatalf("failed to register v1: %v", err)
	}

	// Case A: Compatible evolution (adding optional field)
	v2DefCompatible := `{
		"type": "object",
		"required": ["id"],
		"properties": {
			"id": { "type": "string" },
			"name": { "type": "string" },
			"phone": { "type": "string" }
		}
	}`
	v2, err := reg.RegisterSchema(ctx, &schema.Schema{
		Name:          "customer.updated",
		Definition:    v2DefCompatible,
		Compatibility: schema.CompatibilityBackward,
	})
	if err != nil {
		t.Fatalf("expected compatible v2 to register successfully, got: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("expected version 2, got %d", v2.Version)
	}

	// Case B: Incompatible evolution (adding new required field under BACKWARD compatibility)
	v3DefIncompatible := `{
		"type": "object",
		"required": ["id", "tax_number"],
		"properties": {
			"id": { "type": "string" },
			"name": { "type": "string" },
			"tax_number": { "type": "string" }
		}
	}`
	_, err = reg.RegisterSchema(ctx, &schema.Schema{
		Name:          "customer.updated",
		Definition:    v3DefIncompatible,
		Compatibility: schema.CompatibilityBackward,
	})
	if err == nil {
		t.Fatal("expected error registering incompatible schema version")
	}
	if !errors.Is(err, schema.ErrIncompatibleSchema) {
		t.Fatalf("expected ErrIncompatibleSchema, got: %v", err)
	}

	// Case C: Forcing breaking change with CompatibilityNone
	v3Forced, err := reg.RegisterSchema(ctx, &schema.Schema{
		Name:          "customer.updated",
		Definition:    v3DefIncompatible,
		Compatibility: schema.CompatibilityNone,
	})
	if err != nil {
		t.Fatalf("expected CompatibilityNone to allow schema change, got: %v", err)
	}
	if v3Forced.Version != 3 {
		t.Fatalf("expected version 3, got %d", v3Forced.Version)
	}
}

func TestSchemaMultiTenancy(t *testing.T) {
	reg := schema.NewRegistry(schema.NewMemoryStore())
	ctx := context.Background()

	// Tenant A schema
	_, _ = reg.RegisterSchema(ctx, &schema.Schema{
		TenantID:   "tenant-a",
		Name:       "order.placed",
		Definition: `{"type": "object", "required": ["order_id"]}`,
	})

	// Tenant B schema
	_, _ = reg.RegisterSchema(ctx, &schema.Schema{
		TenantID:   "tenant-b",
		Name:       "order.placed",
		Definition: `{"type": "object", "required": ["transaction_id"]}`,
	})

	// Validate against Tenant A
	errsA, _ := reg.ValidateEvent(ctx, "tenant-a", "order.placed", []byte(`{"order_id": "ord_1"}`))
	if len(errsA) != 0 {
		t.Errorf("tenant-a payload should be valid, got: %v", errsA)
	}

	errsAFail, _ := reg.ValidateEvent(ctx, "tenant-a", "order.placed", []byte(`{"transaction_id": "tx_1"}`))
	if len(errsAFail) == 0 {
		t.Errorf("tenant-a payload missing order_id should fail")
	}

	// Validate against Tenant B
	errsB, _ := reg.ValidateEvent(ctx, "tenant-b", "order.placed", []byte(`{"transaction_id": "tx_1"}`))
	if len(errsB) != 0 {
		t.Errorf("tenant-b payload should be valid, got: %v", errsB)
	}
}
