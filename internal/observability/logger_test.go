package observability

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelInfo)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	logger.Info("hello observability", slog.String("component", "test"))
	out := buf.String()
	if !strings.Contains(out, `"msg":"hello observability"`) {
		t.Errorf("expected msg in log output, got: %s", out)
	}
	if !strings.Contains(out, `"component":"test"`) {
		t.Errorf("expected component attribute in log output, got: %s", out)
	}
}

func TestConfigureLoggerLevels(t *testing.T) {
	levels := []string{"debug", "warn", "warning", "error", "info", "unknown_level"}
	for _, lvl := range levels {
		ConfigureLogger(lvl)
		// Ensure slog default is set and callable
		slog.Info("logging test", slog.String("level", lvl))
	}
}
