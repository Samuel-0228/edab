package schema

import (
	"context"
	"testing"
)

func TestMemoryStoreMethods(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	s1, err := store.Register(ctx, &Schema{
		TenantID:   "tenant-1",
		Name:       "user.event",
		Type:       TypeJSONSchema,
		Definition: `{"type":"object"}`,
	})
	if err != nil {
		t.Fatalf("Register s1 failed: %v", err)
	}

	s2, err := store.Register(ctx, &Schema{
		TenantID:   "tenant-1",
		Name:       "user.event",
		Type:       TypeJSONSchema,
		Definition: `{"type":"object","properties":{"id":{"type":"string"}}}`,
	})
	if err != nil {
		t.Fatalf("Register s2 failed: %v", err)
	}
	if s2.Version != 2 {
		t.Errorf("expected version 2 for s2, got %d", s2.Version)
	}

	// GetByID
	gotByID, err := store.GetByID(ctx, s1.ID)
	if err != nil || gotByID.ID != s1.ID {
		t.Fatalf("GetByID failed: %v", err)
	}

	// GetByVersion
	gotV1, err := store.GetByVersion(ctx, "tenant-1", "user.event", 1)
	if err != nil || gotV1.Version != 1 {
		t.Fatalf("GetByVersion 1 failed: %v", err)
	}
	gotV2, err := store.GetByVersion(ctx, "tenant-1", "user.event", 2)
	if err != nil || gotV2.Version != 2 {
		t.Fatalf("GetByVersion 2 failed: %v", err)
	}

	// List
	list, err := store.List(ctx, "tenant-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("List failed: %v, len=%d", err, len(list))
	}
	if list[0].Version != 2 {
		t.Errorf("expected latest version 2 in list, got %d", list[0].Version)
	}

	// ListVersions
	versions, err := store.ListVersions(ctx, "tenant-1", "user.event")
	if err != nil || len(versions) != 2 {
		t.Fatalf("ListVersions failed: %v, len=%d", err, len(versions))
	}

	// Delete
	if err := store.Delete(ctx, s1.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if err := store.Delete(ctx, "non-existent"); err != ErrSchemaNotFound {
		t.Errorf("expected ErrSchemaNotFound on delete")
	}

	// Registry Store accessor
	reg := NewRegistry(store)
	if reg.Store() != store {
		t.Error("expected reg.Store() to match store")
	}

	// ValidationError Error method
	vErr := ValidationError{Field: "user.id", Message: "must be string"}
	if vErr.Error() == "" {
		t.Error("expected non-empty ValidationError string")
	}
}
