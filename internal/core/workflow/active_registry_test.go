package workflow

import (
	"context"
	"testing"
)

func TestActiveExecutionRegistry(t *testing.T) {
	reg := NewActiveExecutionRegistry()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Empty registry tests
	if reg.IsActive("exec-1") {
		t.Error("expected IsActive to be false")
	}
	if reg.IsPaused("exec-1") {
		t.Error("expected IsPaused to be false")
	}
	if reg.Cancel("exec-1") {
		t.Error("expected Cancel to be false")
	}
	if reg.Pause("exec-1") {
		t.Error("expected Pause to be false")
	}
	if len(reg.ActiveIDs()) != 0 {
		t.Error("expected empty ActiveIDs")
	}

	// Register
	reg.Register("exec-1", cancel)
	if !reg.IsActive("exec-1") {
		t.Error("expected IsActive to be true")
	}
	if len(reg.ActiveIDs()) != 1 {
		t.Errorf("expected 1 active ID, got %d", len(reg.ActiveIDs()))
	}

	// Pause
	if !reg.Pause("exec-1") {
		t.Error("expected Pause to succeed")
	}
	if !reg.IsPaused("exec-1") {
		t.Error("expected IsPaused to be true")
	}

	// Cancel
	if !reg.Cancel("exec-1") {
		t.Error("expected Cancel to succeed")
	}

	// Unregister
	reg.Unregister("exec-1")
	if reg.IsActive("exec-1") {
		t.Error("expected IsActive to be false after Unregister")
	}

	// Nil safety
	var nilReg *ActiveExecutionRegistry
	nilReg.Register("x", nil)
	nilReg.Unregister("x")
	if nilReg.Cancel("x") {
		t.Error("expected false for nilReg.Cancel")
	}
	if nilReg.Pause("x") {
		t.Error("expected false for nilReg.Pause")
	}
	if nilReg.IsPaused("x") {
		t.Error("expected false for nilReg.IsPaused")
	}
	if nilReg.IsActive("x") {
		t.Error("expected false for nilReg.IsActive")
	}
	if nilReg.ActiveIDs() != nil {
		t.Error("expected nil for nilReg.ActiveIDs")
	}
}
