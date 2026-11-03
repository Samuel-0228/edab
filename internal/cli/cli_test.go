package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kerob/pkg/sdk"
)

func TestCLICommands(t *testing.T) {
	mux := http.NewServeMux()

	// Mock workflows
	mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sdk.Workflow{
			{
				ID:      "wf_order",
				Name:    "Order Checkout",
				Enabled: true,
				Trigger: sdk.TriggerConfig{Type: "webhook"},
				Actions: []sdk.ActionStep{{ID: "act_1", Name: "Charge Card"}},
			},
		})
	})

	// Mock executions
	mux.HandleFunc("GET /api/v1/executions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sdk.Execution{
			{
				ID:          "exec_101",
				WorkflowID:  "wf_order",
				Status:      sdk.ExecutionFailed,
				Attempts:    3,
				MaxAttempts: 3,
				DurationMs:  120,
			},
		})
	})

	// Mock execution get
	mux.HandleFunc("GET /api/v1/executions/exec_101", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sdk.Execution{
			ID:          "exec_101",
			WorkflowID:  "wf_order",
			Status:      sdk.ExecutionFailed,
			Attempts:    3,
			MaxAttempts: 3,
			DurationMs:  120,
			Error:       "payment gateway timeout",
			Steps: []sdk.ExecutionStep{
				{
					ID:         "step_charge",
					Name:       "Charge Card",
					StepType:   "action",
					Status:     "failed",
					Error:      "payment gateway timeout",
					DurationMs: 120,
				},
			},
		})
	})

	// Mock execution compensate
	mux.HandleFunc("POST /api/v1/executions/exec_101/compensate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sdk.Execution{
			ID:     "exec_101",
			Status: sdk.ExecutionCompensated,
		})
	})

	// Mock event replay
	mux.HandleFunc("POST /api/v1/events/ev_999/replay", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "replayed", "event_id": "ev_999"})
	})

	// Mock DLQ
	mux.HandleFunc("GET /api/v1/failures", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sdk.DeadLetter{
			{
				ID:          "dlq_505",
				ExecutionID: "exec_101",
				WorkflowID:  "wf_order",
				Attempts:    3,
				Error:       "timeout",
			},
		})
	})

	mux.HandleFunc("POST /api/v1/failures/dlq_505/retry", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sdk.Execution{
			ID:     "exec_102",
			Status: sdk.ExecutionRunning,
		})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	var stdout, stderr bytes.Buffer
	app := &App{
		Out:    &stdout,
		ErrOut: &stderr,
		Config: &Config{
			Endpoint: ts.URL,
			Token:    "test_token",
		},
	}

	// 1. Version command
	stdout.Reset()
	stderr.Reset()
	code := app.Execute([]string{"version"})
	if code != 0 || !strings.Contains(stdout.String(), "Kerob Automation Engine") {
		t.Fatalf("version failed: code=%d out=%s", code, stdout.String())
	}

	// 2. Version in JSON mode
	stdout.Reset()
	code = app.Execute([]string{"version", "--json"})
	if code != 0 || !strings.Contains(stdout.String(), `"version"`) {
		t.Fatalf("version --json failed: code=%d out=%s", code, stdout.String())
	}

	// 3. Workflow validate with valid file
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "valid.json")
	validContent := `{
		"name": "Invoice Pipeline",
		"enabled": true,
		"trigger": {"type": "webhook"},
		"actions": [
			{"id": "step_1", "name": "Notify Slack", "integration": "http", "action_type": "request"}
		]
	}`
	_ = os.WriteFile(validFile, []byte(validContent), 0644)

	stdout.Reset()
	code = app.Execute([]string{"workflow", "validate", validFile})
	if code != 0 || !strings.Contains(stdout.String(), "is valid and passed secret safety scanning") {
		t.Fatalf("workflow validate failed: code=%d out=%s err=%s", code, stdout.String(), stderr.String())
	}

	// 4. Workflow validate with secret violation
	invalidFile := filepath.Join(tmpDir, "invalid.json")
	invalidContent := `{
		"name": "Leaked Key Pipeline",
		"enabled": true,
		"trigger": {"type": "webhook"},
		"actions": [
			{"id": "step_1", "name": "Call S3", "integration": "http", "action_type": "request", "config": {"api_key": "AKIAIOSFODNN7EXAMPLE"}}
		]
	}`
	_ = os.WriteFile(invalidFile, []byte(invalidContent), 0644)

	stderr.Reset()
	code = app.Execute([]string{"workflow", "validate", invalidFile})
	if code == 0 {
		t.Fatal("expected failure on secret violation, got code 0")
	}

	// 5. Workflow list
	stdout.Reset()
	code = app.Execute([]string{"workflow", "list"})
	if code != 0 || !strings.Contains(stdout.String(), "Order Checkout") {
		t.Fatalf("workflow list failed: code=%d out=%s", code, stdout.String())
	}

	// 6. Execution list
	stdout.Reset()
	code = app.Execute([]string{"execution", "list"})
	if code != 0 || !strings.Contains(stdout.String(), "exec_101") {
		t.Fatalf("execution list failed: code=%d out=%s", code, stdout.String())
	}

	// 7. Execution get
	stdout.Reset()
	code = app.Execute([]string{"execution", "get", "exec_101"})
	if code != 0 || !strings.Contains(stdout.String(), "payment gateway timeout") {
		t.Fatalf("execution get failed: code=%d out=%s", code, stdout.String())
	}

	// 8. Execution logs
	stdout.Reset()
	code = app.Execute([]string{"logs", "exec_101"})
	if code != 0 || !strings.Contains(stdout.String(), "Charge Card") {
		t.Fatalf("logs failed: code=%d out=%s", code, stdout.String())
	}

	// 9. Execution compensate
	stdout.Reset()
	code = app.Execute([]string{"execution", "compensate", "exec_101", "--reason", "Customer cancelled"})
	if code != 0 || !strings.Contains(stdout.String(), "compensation rollback completed") {
		t.Fatalf("execution compensate failed: code=%d out=%s", code, stdout.String())
	}

	// 10. Event replay
	stdout.Reset()
	code = app.Execute([]string{"event", "replay", "ev_999"})
	if code != 0 || !strings.Contains(stdout.String(), "queued for automation replay") {
		t.Fatalf("event replay failed: code=%d out=%s", code, stdout.String())
	}

	// 11. DLQ list and retry
	stdout.Reset()
	code = app.Execute([]string{"dlq", "list"})
	if code != 0 || !strings.Contains(stdout.String(), "dlq_505") {
		t.Fatalf("dlq list failed: code=%d out=%s", code, stdout.String())
	}

	stdout.Reset()
	code = app.Execute([]string{"dlq", "retry", "dlq_505"})
	if code != 0 || !strings.Contains(stdout.String(), "re-queued for execution") {
		t.Fatalf("dlq retry failed: code=%d out=%s", code, stdout.String())
	}

	// 12. Shell completion
	stdout.Reset()
	code = app.Execute([]string{"completion", "bash"})
	if code != 0 || !strings.Contains(stdout.String(), "_kerob_completions") {
		t.Fatalf("completion bash failed: code=%d", code)
	}

	stdout.Reset()
	code = app.Execute([]string{"completion", "zsh"})
	if code != 0 || !strings.Contains(stdout.String(), "#compdef kerob") {
		t.Fatalf("completion zsh failed: code=%d", code)
	}

	stdout.Reset()
	code = app.Execute([]string{"completion", "fish"})
	if code != 0 || !strings.Contains(stdout.String(), "complete -c kerob") {
		t.Fatalf("completion fish failed: code=%d", code)
	}
}
