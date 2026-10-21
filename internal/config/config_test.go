package config

import (
	"os"
	"strings"
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

func TestEnvironmentDetection(t *testing.T) {
	tests := []struct {
		env           string
		isProduction  bool
		isDevelopment bool
	}{
		{"development", false, true},
		{"Development", false, true},
		{"dev", false, true},
		{"DEV", false, true},
		{"local", false, true},
		{"", false, true},
		{"production", true, false},
		{"Production", true, false},
		{"PRODUCTION", true, false},
		{"prod", true, false},
		{"PROD", true, false},
		{"staging", false, false},
		{"test", false, false},
	}

	for _, tt := range tests {
		cfg := &Config{Environment: tt.env}
		if got := cfg.IsProduction(); got != tt.isProduction {
			t.Errorf("Config{Environment: %q}.IsProduction() = %v, want %v", tt.env, got, tt.isProduction)
		}
		if got := cfg.IsDevelopment(); got != tt.isDevelopment {
			t.Errorf("Config{Environment: %q}.IsDevelopment() = %v, want %v", tt.env, got, tt.isDevelopment)
		}
	}
}

func TestValidate_Development(t *testing.T) {
	baseDevConfig := func() *Config {
		return &Config{
			DatabaseURL:       "postgres://localhost:5432/kerob",
			Environment:       "development",
			APIToken:          "",
			WorkerConcurrency: 4,
			MaxRetries:        3,
		}
	}

	t.Run("valid dev without API_TOKEN", func(t *testing.T) {
		cfg := baseDevConfig()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected development mode to validate without API_TOKEN, got error: %v", err)
		}
	})

	t.Run("valid dev with API_TOKEN", func(t *testing.T) {
		cfg := baseDevConfig()
		cfg.APIToken = "custom-dev-token"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected development mode to validate with API_TOKEN, got error: %v", err)
		}
	})

	t.Run("valid dev with short API_TOKEN", func(t *testing.T) {
		cfg := baseDevConfig()
		cfg.APIToken = "short"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected development mode to permit short dev token, got error: %v", err)
		}
	})

	t.Run("dev mode aliases", func(t *testing.T) {
		for _, env := range []string{"dev", "DEV", "local", ""} {
			cfg := baseDevConfig()
			cfg.Environment = env
			if err := cfg.Validate(); err != nil {
				t.Errorf("expected env %q to validate without API_TOKEN, got: %v", env, err)
			}
		}
	})
}

func TestValidate_Production(t *testing.T) {
	baseProdConfig := func() *Config {
		return &Config{
			DatabaseURL:       "postgres://localhost:5432/kerob",
			Environment:       "production",
			APIToken:          "a-valid-production-secure-token-32b",
			WorkerConcurrency: 4,
			MaxRetries:        3,
		}
	}

	t.Run("production with secure token succeeds", func(t *testing.T) {
		cfg := baseProdConfig()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected production to validate with secure token, got error: %v", err)
		}
	})

	t.Run("production missing API_TOKEN fails", func(t *testing.T) {
		cfg := baseProdConfig()
		cfg.APIToken = ""
		err := cfg.Validate()
		if err == nil {
			t.Fatalf("expected error when API_TOKEN is empty in production, got nil")
		}
		if !strings.Contains(err.Error(), "API_TOKEN is required in production") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("production whitespace API_TOKEN fails", func(t *testing.T) {
		cfg := baseProdConfig()
		cfg.APIToken = "    "
		err := cfg.Validate()
		if err == nil {
			t.Fatalf("expected error when API_TOKEN is whitespace only in production, got nil")
		}
		if !strings.Contains(err.Error(), "API_TOKEN is required in production") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("production case insensitive environment fails without token", func(t *testing.T) {
		for _, env := range []string{"Production", "PRODUCTION", "prod", "PROD"} {
			cfg := baseProdConfig()
			cfg.Environment = env
			cfg.APIToken = ""
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error for env %q without token, got nil", env)
			}
		}
	})

	t.Run("production placeholder API_TOKEN rejected", func(t *testing.T) {
		placeholders := []string{
			"your-random-production-api-token-here",
			"your-secure-admin-token",
			"changeme",
			"admin",
			"password",
			"secret",
		}
		for _, ph := range placeholders {
			cfg := baseProdConfig()
			cfg.APIToken = ph
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected placeholder %q to be rejected in production, got nil", ph)
			}
		}
	})

	t.Run("production short API_TOKEN rejected", func(t *testing.T) {
		cfg := baseProdConfig()
		cfg.APIToken = "too-short-token" // 15 characters
		err := cfg.Validate()
		if err == nil {
			t.Fatalf("expected short token to be rejected in production, got nil")
		}
		if !strings.Contains(err.Error(), "at least 16 characters") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestValidate_CommonConstraints(t *testing.T) {
	cfg := &Config{
		DatabaseURL:       "",
		Environment:       "development",
		WorkerConcurrency: 4,
		MaxRetries:        3,
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Errorf("expected DATABASE_URL error, got: %v", err)
	}

	cfg.DatabaseURL = "postgres://localhost:5432/kerob"
	cfg.WorkerConcurrency = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "WORKER_CONCURRENCY") {
		t.Errorf("expected WORKER_CONCURRENCY error, got: %v", err)
	}

	cfg.WorkerConcurrency = 1
	cfg.MaxRetries = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "MAX_RETRIES") {
		t.Errorf("expected MAX_RETRIES error, got: %v", err)
	}
}
