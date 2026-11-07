package cli

import (
	"bytes"
	"testing"
)

func TestOutputFormatter(t *testing.T) {
	var out, errOut bytes.Buffer

	// Human mode
	fHuman := NewFormatter(&out, &errOut, false)
	_ = fHuman.PrintJSON(map[string]string{"foo": "bar"})
	fHuman.PrintTable([]string{"Name", "Status"}, [][]string{{"Workflow A", "Active"}})
	fHuman.PrintSuccess("done %s", "task")
	fHuman.PrintInfo("info %s", "item")
	fHuman.PrintWarning("warning %s", "flag")
	fHuman.PrintError("error %s", "failure")

	if out.Len() == 0 || errOut.Len() == 0 {
		t.Error("expected non-empty output in human mode")
	}

	out.Reset()
	errOut.Reset()

	// JSON mode
	fJSON := NewFormatter(&out, &errOut, true)
	fJSON.PrintTable([]string{"Name", "Status"}, [][]string{{"Workflow A", "Active"}})
	fJSON.PrintSuccess("done %s", "task")
	fJSON.PrintInfo("info %s", "item")
	fJSON.PrintWarning("warning %s", "flag")
	fJSON.PrintError("error %s", "failure")

	if out.Len() == 0 {
		t.Error("expected non-empty output in JSON mode")
	}
}

func TestConfigLoadAndPath(t *testing.T) {
	path := ConfigPath()
	if path == "" {
		t.Error("expected non-empty ConfigPath")
	}

	cfg, err := LoadConfig()
	if err != nil || cfg == nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
}

func TestAppRunHelpAndVersion(t *testing.T) {
	code := Run([]string{"--help"})
	if code != 0 {
		t.Errorf("expected exit code 0 for --help, got %d", code)
	}

	code = Run([]string{"version"})
	if code != 0 {
		t.Errorf("expected exit code 0 for version, got %d", code)
	}
}
