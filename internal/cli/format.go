package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// OutputFormatter manages output formatting based on JSON or human-readable mode.
type OutputFormatter struct {
	Out      io.Writer
	ErrOut   io.Writer
	JSONMode bool
}

// NewFormatter constructs an OutputFormatter.
func NewFormatter(out, errOut io.Writer, jsonMode bool) *OutputFormatter {
	return &OutputFormatter{
		Out:      out,
		ErrOut:   errOut,
		JSONMode: jsonMode,
	}
}

// PrintJSON prints a value as indented JSON.
func (f *OutputFormatter) PrintJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format JSON: %w", err)
	}
	_, err = fmt.Fprintln(f.Out, string(data))
	return err
}

// PrintTable renders rows using an aligned tabwriter table.
func (f *OutputFormatter) PrintTable(headers []string, rows [][]string) {
	if f.JSONMode {
		var list []map[string]string
		for _, row := range rows {
			item := make(map[string]string)
			for i, h := range headers {
				key := strings.ToLower(strings.ReplaceAll(h, " ", "_"))
				val := ""
				if i < len(row) {
					val = row[i]
				}
				item[key] = val
			}
			list = append(list, item)
		}
		_ = f.PrintJSON(list)
		return
	}

	w := tabwriter.NewWriter(f.Out, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		_, _ = fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	_ = w.Flush()
}

// PrintSuccess prints a success message.
func (f *OutputFormatter) PrintSuccess(format string, a ...any) {
	if f.JSONMode {
		msg := fmt.Sprintf(format, a...)
		_ = f.PrintJSON(map[string]any{"status": "success", "message": msg})
		return
	}
	_, _ = fmt.Fprintf(f.Out, "✓ "+format+"\n", a...)
}

// PrintError prints an error message to standard error.
func (f *OutputFormatter) PrintError(format string, a ...any) {
	if f.JSONMode {
		msg := fmt.Sprintf(format, a...)
		_ = f.PrintJSON(map[string]any{"status": "error", "error": msg})
		return
	}
	_, _ = fmt.Fprintf(f.ErrOut, "✗ "+format+"\n", a...)
}
