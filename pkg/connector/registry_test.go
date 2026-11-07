package connector

import (
	"testing"
)

type mockConnector struct {
	id string
}

func (m *mockConnector) Manifest() Manifest {
	return Manifest{
		ID:   m.id,
		Name: "Mock " + m.id,
	}
}

func (m *mockConnector) ValidateConfig(config map[string]any) error {
	return nil
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected registry to be non-nil")
	}

	// Register nil
	if err := r.Register(nil); err == nil {
		t.Error("expected error registering nil")
	}

	// Register empty ID
	if err := r.Register(&mockConnector{id: ""}); err == nil {
		t.Error("expected error registering empty ID")
	}

	c1 := &mockConnector{id: "conn-1"}
	c2 := &mockConnector{id: "conn-2"}

	if err := r.Register(c1); err != nil {
		t.Fatalf("failed to register c1: %v", err)
	}
	if err := r.Register(c2); err != nil {
		t.Fatalf("failed to register c2: %v", err)
	}

	// Has and Get
	if !r.Has("conn-1") {
		t.Error("expected r.Has(conn-1) to be true")
	}
	if r.Has("conn-unknown") {
		t.Error("expected r.Has(conn-unknown) to be false")
	}

	got, ok := r.Get("conn-1")
	if !ok || got.Manifest().ID != "conn-1" {
		t.Errorf("unexpected connector from Get: %+v", got)
	}

	// List
	list := r.List()
	if len(list) != 2 {
		t.Errorf("expected 2 connectors in list, got %d", len(list))
	}

	// ListManifests
	manifests := r.ListManifests()
	if len(manifests) != 2 {
		t.Errorf("expected 2 manifests, got %d", len(manifests))
	}

	// Global functions
	def := Default()
	if def == nil {
		t.Fatal("expected default registry to be non-nil")
	}
	if err := Register(c1); err != nil {
		t.Fatalf("global Register failed: %v", err)
	}
	if _, ok := Get("conn-1"); !ok {
		t.Error("global Get failed")
	}
	if len(List()) == 0 {
		t.Error("global List returned empty")
	}
	if len(ListManifests()) == 0 {
		t.Error("global ListManifests returned empty")
	}
}
