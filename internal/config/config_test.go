package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// Clear any relevant environment variables
	envVars := []string{
		"DATABASE_URL", "PORT", "SERVER_PORT", "WORKER_POLL_INTERVAL",
		"WORKER_CONCURRENCY", "MAX_RETRIES", "INITIAL_BACKOFF", "MAX_BACKOFF",
		"LOG_LEVEL", "ENVIRONMENT", "API_TOKEN",
	}

	for _, k := range envVars {
		_ = os.Unsetenv(k)
	}

	cfg := Load()

	if cfg.ServerPort != "8080" {
		t.Errorf("expected default ServerPort 8080, got %s", cfg.ServerPort)
	}
	if cfg.WorkerConcurrency != 4 {
		t.Errorf("expected default WorkerConcurrency 4, got %d", cfg.WorkerConcurrency)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected default MaxRetries 3, got %d", cfg.MaxRetries)
	}
	if cfg.WorkerPollInterval != 1*time.Second {
		t.Errorf("expected default WorkerPollInterval 1s, got %v", cfg.WorkerPollInterval)
	}
	if cfg.InitialBackoff != 2*time.Second {
		t.Errorf("expected default InitialBackoff 2s, got %v", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != 1*time.Minute {
		t.Errorf("expected default MaxBackoff 1m, got %v", cfg.MaxBackoff)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default LogLevel info, got %s", cfg.LogLevel)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected default Environment development, got %s", cfg.Environment)
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("WORKER_CONCURRENCY", "8")
	t.Setenv("MAX_RETRIES", "5")
	t.Setenv("WORKER_POLL_INTERVAL", "500ms")
	t.Setenv("INITIAL_BACKOFF", "100ms")
	t.Setenv("MAX_BACKOFF", "5s")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("ENVIRONMENT", "production")

	cfg := Load()

	if cfg.ServerPort != "9090" {
		t.Errorf("expected ServerPort 9090, got %s", cfg.ServerPort)
	}
	if cfg.WorkerConcurrency != 8 {
		t.Errorf("expected WorkerConcurrency 8, got %d", cfg.WorkerConcurrency)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("expected MaxRetries 5, got %d", cfg.MaxRetries)
	}
	if cfg.WorkerPollInterval != 500*time.Millisecond {
		t.Errorf("expected WorkerPollInterval 500ms, got %v", cfg.WorkerPollInterval)
	}
	if cfg.InitialBackoff != 100*time.Millisecond {
		t.Errorf("expected InitialBackoff 100ms, got %v", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != 5*time.Second {
		t.Errorf("expected MaxBackoff 5s, got %v", cfg.MaxBackoff)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel debug, got %s", cfg.LogLevel)
	}
	if cfg.Environment != "production" {
		t.Errorf("expected Environment production, got %s", cfg.Environment)
	}
}

func TestInvalidNumberOrDurationFallback(t *testing.T) {
	t.Setenv("WORKER_CONCURRENCY", "not-a-number")
	t.Setenv("WORKER_POLL_INTERVAL", "not-a-duration")

	cfg := Load()

	if cfg.WorkerConcurrency != 4 {
		t.Errorf("expected fallback WorkerConcurrency 4, got %d", cfg.WorkerConcurrency)
	}

	if cfg.WorkerPollInterval != 1*time.Second {
		t.Errorf("expected fallback WorkerPollInterval 1s, got %v", cfg.WorkerPollInterval)
	}
}
