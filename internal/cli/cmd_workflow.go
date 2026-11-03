package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"kerob/internal/cli/gitops"
	"kerob/pkg/sdk"
)

func runWorkflow(ctx context.Context, client *sdk.Client, formatter *OutputFormatter, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing workflow subcommand. Available: list, get, validate, import, export, deploy, publish, rollback, diff")
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list":
		wfs, err := client.ListWorkflows(ctx)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(wfs)
		}
		headers := []string{"ID", "NAME", "TRIGGER", "STEPS", "ENABLED"}
		var rows [][]string
		for _, w := range wfs {
			rows = append(rows, []string{
				w.ID,
				w.Name,
				w.Trigger.Type,
				strconv.Itoa(len(w.Actions)),
				strconv.FormatBool(w.Enabled),
			})
		}
		formatter.PrintTable(headers, rows)
		return nil

	case "get":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob workflow get <id>")
		}
		wf, err := client.GetWorkflow(ctx, subArgs[0])
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(wf)
		}
		_, _ = fmt.Fprintf(formatter.Out, "Workflow ID:   %s\n", wf.ID)
		_, _ = fmt.Fprintf(formatter.Out, "Name:          %s\n", wf.Name)
		_, _ = fmt.Fprintf(formatter.Out, "Description:   %s\n", wf.Description)
		_, _ = fmt.Fprintf(formatter.Out, "Enabled:       %t\n", wf.Enabled)
		_, _ = fmt.Fprintf(formatter.Out, "Trigger:       %s\n", wf.Trigger.Type)
		_, _ = fmt.Fprintf(formatter.Out, "Total Steps:   %d\n", len(wf.Actions))
		for i, a := range wf.Actions {
			_, _ = fmt.Fprintf(formatter.Out, "  [%d] %s (%s / %s)\n", i+1, a.Name, a.Integration, a.ActionType)
		}
		return nil

	case "validate":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob workflow validate <file>")
		}
		file := subArgs[0]
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}
		wf, err := gitops.ParseWorkflowFile(data)
		if err != nil {
			formatter.PrintError("Parse failed: %s", err.Error())
			return err
		}
		errs := gitops.ValidateWorkflow(wf)
		if len(errs) > 0 {
			if formatter.JSONMode {
				return formatter.PrintJSON(map[string]any{
					"valid":  false,
					"file":   file,
					"errors": errs,
				})
			}
			formatter.PrintError("Validation failed for %s (%d issue(s)):", file, len(errs))
			for _, e := range errs {
				fatalPrefix := "[WARN]"
				if e.Fatal {
					fatalPrefix = "[FATAL]"
				}
				_, _ = fmt.Fprintf(formatter.ErrOut, "  %s %s\n", fatalPrefix, e.Error())
			}
			return fmt.Errorf("validation failed with %d error(s)", len(errs))
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(map[string]any{
				"valid": true,
				"file":  file,
				"name":  wf.Name,
				"steps": len(wf.Actions),
			})
		}
		formatter.PrintSuccess("Workflow specification %q is valid and passed secret safety scanning.", file)
		return nil

	case "import":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob workflow import <file>")
		}
		data, err := os.ReadFile(subArgs[0])
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", subArgs[0], err)
		}
		imported, err := client.ImportWorkflow(ctx, data)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(imported)
		}
		formatter.PrintSuccess("Imported workflow %s (%s) with %d step(s)", imported.Name, imported.ID, len(imported.Actions))
		return nil

	case "export":
		fs := flag.NewFlagSet("workflow export", flag.ContinueOnError)
		fs.SetOutput(formatter.ErrOut)
		format := fs.String("format", "json", "Export format (json or yaml)")
		outFile := fs.String("out", "", "Optional output file path")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		if len(fs.Args()) == 0 {
			return fmt.Errorf("usage: kerob workflow export <id> [--format json|yaml] [--out file]")
		}
		id := fs.Args()[0]
		data, err := client.ExportWorkflow(ctx, id, *format)
		if err != nil {
			return err
		}
		if *outFile != "" {
			if err := os.WriteFile(*outFile, data, 0644); err != nil {
				return fmt.Errorf("failed to write output file: %w", err)
			}
			formatter.PrintSuccess("Exported workflow %s to %s", id, *outFile)
			return nil
		}
		_, _ = fmt.Fprintln(formatter.Out, string(data))
		return nil

	case "deploy":
		fs := flag.NewFlagSet("workflow deploy", flag.ContinueOnError)
		fs.SetOutput(formatter.ErrOut)
		dryRun := fs.Bool("dry-run", false, "Preview planned deployment changes without modifying remote state")
		showDiff := fs.Bool("diff", false, "Display structural differences for updated workflows")
		requireApproval := fs.Bool("require-approval", false, "Enforce explicit confirmation before applying changes")
		approve := fs.Bool("approve", false, "Provide explicit confirmation for deployment approval")
		rollbackOnFailure := fs.Bool("rollback-on-failure", true, "Automatically rollback modified workflows if deployment fails")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		if len(fs.Args()) == 0 {
			return fmt.Errorf("usage: kerob workflow deploy <path> [--dry-run] [--diff] [--require-approval] [--approve]")
		}
		targetPath := fs.Args()[0]

		report, err := gitops.DeployPath(ctx, client, targetPath, gitops.DeployOptions{
			DryRun:            *dryRun,
			RequireApproval:   *requireApproval,
			Approved:          *approve,
			RollbackOnFailure: *rollbackOnFailure,
		})
		if err != nil {
			return err
		}

		if formatter.JSONMode {
			return formatter.PrintJSON(report)
		}

		// Human formatted report
		headers := []string{"FILE", "WORKFLOW ID", "NAME", "ACTION", "DIFF / STATUS"}
		var rows [][]string
		for _, w := range report.Workflows {
			statusSummary := w.Diff.Unified
			if w.Error != "" {
				statusSummary = "ERROR: " + w.Error
			} else if len(w.Diff.Changes) > 0 {
				statusSummary = strings.Join(w.Diff.Changes, "; ")
			}
			rows = append(rows, []string{
				w.FilePath,
				w.WorkflowID,
				w.Name,
				strings.ToUpper(w.Action),
				statusSummary,
			})
		}
		formatter.PrintTable(headers, rows)

		if *showDiff {
			for _, w := range report.Workflows {
				if w.Diff.HasChanges && w.Diff.Unified != "" {
					_, _ = fmt.Fprintf(formatter.Out, "\n--- Diff for %s ---\n%s\n", w.Name, w.Diff.Unified)
				}
			}
		}

		if report.DryRun {
			formatter.PrintSuccess("[DRY RUN] %s", report.Summary)
		} else if report.Success {
			formatter.PrintSuccess("%s", report.Summary)
		} else {
			formatter.PrintError("Deployment failed: %s", report.Summary)
			return fmt.Errorf("deployment completed with errors")
		}
		return nil

	case "publish":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob workflow publish <id>")
		}
		id := subArgs[0]
		updated, err := client.PatchWorkflow(ctx, id, map[string]any{"enabled": true})
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(updated)
		}
		formatter.PrintSuccess("Workflow %s published and enabled for live execution.", id)
		return nil

	case "rollback":
		if len(subArgs) < 2 {
			return fmt.Errorf("usage: kerob workflow rollback <id> <version>")
		}
		id := subArgs[0]
		verNum, err := strconv.Atoi(subArgs[1])
		if err != nil {
			return fmt.Errorf("invalid version %q: must be integer", subArgs[1])
		}
		restored, err := client.RollbackWorkflow(ctx, id, verNum)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(restored)
		}
		formatter.PrintSuccess("Workflow %s rolled back to version %d snapshot.", id, verNum)
		return nil

	case "diff":
		if len(subArgs) < 3 {
			return fmt.Errorf("usage: kerob workflow diff <id> <from_version> <to_version>")
		}
		id := subArgs[0]
		fromVer, err1 := strconv.Atoi(subArgs[1])
		toVer, err2 := strconv.Atoi(subArgs[2])
		if err1 != nil || err2 != nil {
			return fmt.Errorf("version arguments must be integers")
		}
		diff, err := client.DiffWorkflowVersions(ctx, id, fromVer, toVer)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(diff)
		}
		_, _ = fmt.Fprintf(formatter.Out, "Diff for Workflow %s (v%d -> v%d):\n", id, fromVer, toVer)
		_, _ = fmt.Fprintf(formatter.Out, "From snapshot:\n%s\n\nTo snapshot:\n%s\n", string(diff.From), string(diff.To))
		return nil

	default:
		return fmt.Errorf("unknown workflow subcommand %q. Available: list, get, validate, import, export, deploy, publish, rollback, diff", sub)
	}
}
