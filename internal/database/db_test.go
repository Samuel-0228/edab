package database

import (
	"strings"
	"testing"
)

func TestConnectEmptyURL(t *testing.T) {
	_, err := Connect(Config{URL: ""})
	if err == nil || !strings.Contains(err.Error(), "database URL is required") {
		t.Fatalf("expected 'database URL is required' error, got %v", err)
	}
}

func TestConnectInvalidURL(t *testing.T) {
	// A non-routable dummy address should fail to ping within timeout
	_, err := Connect(Config{URL: "postgres://invalid:user@127.0.0.1:54329/nonexistent?sslmode=disable&connect_timeout=1"})
	if err == nil {
		t.Fatal("expected connection failure for invalid database address, got nil")
	}
}
