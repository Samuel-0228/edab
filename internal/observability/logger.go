package observability

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// ConfigureLogger installs the process-wide JSON logger used by all layers.
func ConfigureLogger(levelName string) {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(levelName)) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(NewLogger(os.Stdout, level))
}

func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
