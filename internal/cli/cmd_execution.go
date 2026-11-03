package cli

import (
	"context"
	"flag"
	"fmt"
	"time"

	"kerob/pkg/sdk"
)

func runExecution(ctx context.Context, client *sdk.Client, formatter *OutputFormatter, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing execution subcommand. Available: list, get, retry, pause, resume, cancel, compensate")
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list":
		fs := flag.NewFlagSet("execution list", flag.ContinueOnError)
		fs.SetOutput(formatter.ErrOut)
		wfID := fs.String("workflow-id", "", "Filter by workflow ID")
		status := fs.String("status", "", "Filter by execution status")
		limit := fs.Int("limit", 50, "Limit number of records returned")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}

		filter := sdk.ExecutionFilter{
			WorkflowID: *wfID,
			Status:     *status,
			Limit:      *limit,
		}
		execs, err := client.ListExecutions(ctx, filter)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(execs)
		}
		headers := []string{"EXECUTION ID", "WORKFLOW ID", "STATUS", "ATTEMPTS", "DURATION", "STARTED AT"}
		var rows [][]string
		for _, e := range execs {
			dur := fmt.Sprintf("%dms", e.DurationMs)
			started := e.StartedAt.Format(time.RFC3339)
			rows = append(rows, []string{
				e.ID,
				e.WorkflowID,
				string(e.Status),
				fmt.Sprintf("%d/%d", e.Attempts, e.MaxAttempts),
				dur,
				started,
			})
		}
		formatter.PrintTable(headers, rows)
		return nil

	case "get":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob execution get <id>")
		}
		id := subArgs[0]
		exec, err := client.GetExecution(ctx, id)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(exec)
		}
		_, _ = fmt.Fprintf(formatter.Out, "Execution ID: %s\n", exec.ID)
		_, _ = fmt.Fprintf(formatter.Out, "Workflow ID:  %s\n", exec.WorkflowID)
		_, _ = fmt.Fprintf(formatter.Out, "Event ID:     %s\n", exec.EventID)
		_, _ = fmt.Fprintf(formatter.Out, "Status:       %s\n", exec.Status)
		_, _ = fmt.Fprintf(formatter.Out, "Attempts:     %d / %d\n", exec.Attempts, exec.MaxAttempts)
		_, _ = fmt.Fprintf(formatter.Out, "Duration:     %d ms\n", exec.DurationMs)
		_, _ = fmt.Fprintf(formatter.Out, "Started At:   %s\n", exec.StartedAt.Format(time.RFC3339))
		if exec.Error != "" {
			_, _ = fmt.Fprintf(formatter.Out, "Error:        %s\n", exec.Error)
		}
		if len(exec.Steps) > 0 {
			_, _ = fmt.Fprintln(formatter.Out, "\nGranular Stages Timeline:")
			headers := []string{"STAGE", "TYPE", "STATUS", "DURATION", "ERROR"}
			var rows [][]string
			for _, s := range exec.Steps {
				rows = append(rows, []string{
					s.Name,
					s.StepType,
					s.Status,
					fmt.Sprintf("%dms", s.DurationMs),
					s.Error,
				})
			}
			formatter.PrintTable(headers, rows)
		}
		return nil

	case "retry":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob execution retry <id>")
		}
		exec, err := client.RetryExecution(ctx, subArgs[0])
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(exec)
		}
		formatter.PrintSuccess("Retry scheduled for execution %s (status: %s)", exec.ID, exec.Status)
		return nil

	case "pause":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob execution pause <id>")
		}
		exec, err := client.PauseExecution(ctx, subArgs[0])
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(exec)
		}
		formatter.PrintSuccess("Execution %s paused.", exec.ID)
		return nil

	case "resume":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob execution resume <id>")
		}
		exec, err := client.ResumeExecution(ctx, subArgs[0])
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(exec)
		}
		formatter.PrintSuccess("Execution %s resumed.", exec.ID)
		return nil

	case "cancel":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob execution cancel <id>")
		}
		exec, err := client.CancelExecution(ctx, subArgs[0])
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(exec)
		}
		formatter.PrintSuccess("Execution %s cancelled.", exec.ID)
		return nil

	case "compensate":
		fs := flag.NewFlagSet("execution compensate", flag.ContinueOnError)
		fs.SetOutput(formatter.ErrOut)
		reason := fs.String("reason", "Operator requested rollback via CLI", "Reason for saga compensation rollback")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		if len(fs.Args()) == 0 {
			return fmt.Errorf("usage: kerob execution compensate <id> [--reason <reason>]")
		}
		id := fs.Args()[0]
		exec, err := client.CompensateExecution(ctx, id, *reason)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(exec)
		}
		formatter.PrintSuccess("Saga compensation rollback completed for execution %s (status: %s).", exec.ID, exec.Status)
		return nil

	default:
		return fmt.Errorf("unknown execution subcommand %q. Available: list, get, retry, pause, resume, cancel, compensate", sub)
	}
}

func runLogs(ctx context.Context, client *sdk.Client, formatter *OutputFormatter, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: kerob logs <execution_id>")
	}
	execID := args[0]
	steps, err := client.GetExecutionLogs(ctx, execID)
	if err != nil {
		return err
	}
	if formatter.JSONMode {
		return formatter.PrintJSON(steps)
	}
	if len(steps) == 0 {
		formatter.PrintError("No granular stage logs found for execution %s", execID)
		return nil
	}
	_, _ = fmt.Fprintf(formatter.Out, "Execution Logs for %s (%d stages recorded):\n\n", execID, len(steps))
	for i, s := range steps {
		statusIndicator := "✓"
		if s.Status == "failed" {
			statusIndicator = "✗"
		} else if s.Status == "skipped" {
			statusIndicator = "-"
		}
		_, _ = fmt.Fprintf(formatter.Out, "[%s] %d. %s (%s) — %d ms\n", statusIndicator, i+1, s.Name, s.StepType, s.DurationMs)
		if s.Error != "" {
			_, _ = fmt.Fprintf(formatter.Out, "    Error: %s\n", s.Error)
		}
		if len(s.Output) > 0 && string(s.Output) != "{}" && string(s.Output) != "null" {
			_, _ = fmt.Fprintf(formatter.Out, "    Output: %s\n", string(s.Output))
		}
	}
	return nil
}
