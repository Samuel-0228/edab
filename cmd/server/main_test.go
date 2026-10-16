package main

import (
	"testing"
)

func TestSetupLogger(t *testing.T) {
	levels := []string{"debug", "info", "warn", "warning", "error", "unknown"}
	for _, lvl := range levels {
		// Verify setupLogger runs without panic for all levels
		setupLogger(lvl)
	}
}
