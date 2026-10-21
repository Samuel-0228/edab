package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kerob/internal/config"
)

// getRepoRoot locates the root directory containing docker-compose.yml
func getRepoRoot(t *testing.T) string {
	t.Helper()
	// Tests package is in <root>/tests
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	// If run from <root>/tests, root is ..
	// If run from <root>, root is .
	candidates := []string{wd, filepath.Dir(wd), filepath.Join(wd, "..")}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "docker-compose.yml")); err == nil {
			return c
		}
	}
	t.Fatalf("could not locate docker-compose.yml starting from %s", wd)
	return ""
}

func hasDocker(t *testing.T) bool {
	t.Helper()
	_, err := exec.LookPath("docker")
	return err == nil
}

func TestDockerCompose_DevelopmentModeWithoutToken(t *testing.T) {
	if !hasDocker(t) {
		t.Skip("docker CLI not found; skipping CLI invocation test")
	}

	root := getRepoRoot(t)

	// Clean environment without API_TOKEN
	cmd := exec.Command("docker", "compose", "-f", "docker-compose.yml", "config")
	cmd.Dir = root
	// Filter out API_TOKEN and force clean environment
	env := make([]string, 0)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "API_TOKEN=") && !strings.HasPrefix(e, "ENVIRONMENT=") {
			env = append(env, e)
		}
	}
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("docker compose config should succeed in development without API_TOKEN: %v, stderr: %s", err, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "ENVIRONMENT: development") {
		t.Errorf("expected default ENVIRONMENT to be development in rendered compose output, got:\n%s", output)
	}
}

func TestDockerCompose_EnvExampleCompatibility(t *testing.T) {
	if !hasDocker(t) {
		t.Skip("docker CLI not found; skipping CLI invocation test")
	}

	root := getRepoRoot(t)

	cmd := exec.Command("docker", "compose", "--env-file", ".env.example", "config", "-q")
	cmd.Dir = root
	// Clean environment without API_TOKEN
	env := make([]string, 0)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "API_TOKEN=") && !strings.HasPrefix(e, "ENVIRONMENT=") {
			env = append(env, e)
		}
	}
	cmd.Env = env

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("docker compose with .env.example should validate cleanly: %v, stderr: %s", err, stderr.String())
	}
}

func TestDockerCompose_ProductionOverlayRequiresToken(t *testing.T) {
	if !hasDocker(t) {
		t.Skip("docker CLI not found; skipping CLI invocation test")
	}

	root := getRepoRoot(t)

	// Run prod overlay without API_TOKEN
	cmd := exec.Command("docker", "compose", "-f", "docker-compose.yml", "-f", "docker-compose.prod.yml", "config", "-q")
	cmd.Dir = root
	env := make([]string, 0)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "API_TOKEN=") && !strings.HasPrefix(e, "ENVIRONMENT=") {
			env = append(env, e)
		}
	}
	cmd.Env = env

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected production overlay to fail without API_TOKEN, but it succeeded")
	}
	if !strings.Contains(stderr.String(), "API_TOKEN") {
		t.Errorf("expected stderr to mention missing API_TOKEN, got: %s", stderr.String())
	}
}

func TestDockerCompose_ProductionOverlayWithValidCredentials(t *testing.T) {
	if !hasDocker(t) {
		t.Skip("docker CLI not found; skipping CLI invocation test")
	}

	root := getRepoRoot(t)

	cmd := exec.Command("docker", "compose", "-f", "docker-compose.yml", "-f", "docker-compose.prod.yml", "config", "-q")
	cmd.Dir = root
	env := append(os.Environ(),
		"API_TOKEN=super-secure-production-token-12345",
		"KEROB_ENCRYPTION_KEY=super-secure-production-encryption-key-32b",
	)
	cmd.Env = env

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("expected production overlay with valid credentials to succeed: %v, stderr: %s", err, stderr.String())
	}
}

func TestEnvironmentLifecycle_DevAndProdConsistency(t *testing.T) {
	// 1. In development, API_TOKEN is optional
	t.Run("development mode validates without API_TOKEN", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "development")
		t.Setenv("API_TOKEN", "")
		t.Setenv("DATABASE_URL", "postgres://kerob:password@localhost:5432/kerob")

		cfg := config.Load()
		if !cfg.IsDevelopment() {
			t.Fatalf("expected IsDevelopment() to be true")
		}
		if cfg.IsProduction() {
			t.Fatalf("expected IsProduction() to be false")
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected Validate() to succeed in development without API_TOKEN, got: %v", err)
		}
	})

	// 2. In production, API_TOKEN is required and must be secure
	t.Run("production mode rejects empty API_TOKEN", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "production")
		t.Setenv("API_TOKEN", "")
		t.Setenv("DATABASE_URL", "postgres://kerob:password@localhost:5432/kerob")

		cfg := config.Load()
		if !cfg.IsProduction() {
			t.Fatalf("expected IsProduction() to be true")
		}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected Validate() to fail in production without API_TOKEN")
		}
	})

	t.Run("production mode rejects insecure/placeholder API_TOKEN", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "production")
		t.Setenv("API_TOKEN", "your-random-production-api-token-here")
		t.Setenv("DATABASE_URL", "postgres://kerob:password@localhost:5432/kerob")

		cfg := config.Load()
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected Validate() to fail in production with placeholder API_TOKEN")
		}
	})

	t.Run("production mode succeeds with secure API_TOKEN", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "production")
		t.Setenv("API_TOKEN", "my-secure-production-api-token-67890")
		t.Setenv("DATABASE_URL", "postgres://kerob:password@localhost:5432/kerob")

		cfg := config.Load()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected Validate() to succeed with secure token, got: %v", err)
		}
	})
}
