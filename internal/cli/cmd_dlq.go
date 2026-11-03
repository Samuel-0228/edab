package cli

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"time"

	"kerob/pkg/sdk"
)

func runDLQ(ctx context.Context, client *sdk.Client, formatter *OutputFormatter, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing dlq subcommand. Available: list, retry, discard")
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list":
		fs := flag.NewFlagSet("dlq list", flag.ContinueOnError)
		fs.SetOutput(formatter.ErrOut)
		includeDiscarded := fs.Bool("include-discarded", false, "Include previously discarded records")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}

		failures, err := client.ListFailures(ctx, *includeDiscarded)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(failures)
		}
		headers := []string{"FAILURE ID", "EXECUTION ID", "WORKFLOW ID", "ATTEMPTS", "FAILED AT", "ERROR"}
		var rows [][]string
		for _, f := range failures {
			rows = append(rows, []string{
				f.ID,
				f.ExecutionID,
				f.WorkflowID,
				strconv.Itoa(f.Attempts),
				f.FailedAt.Format(time.RFC3339),
				f.Error,
			})
		}
		formatter.PrintTable(headers, rows)
		return nil

	case "retry":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob dlq retry <failure_id>")
		}
		id := subArgs[0]
		exec, err := client.RetryFailure(ctx, id)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(exec)
		}
		formatter.PrintSuccess("Dead-letter failure %s re-queued for execution (run: %s).", id, exec.ID)
		return nil

	case "discard":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob dlq discard <failure_id>")
		}
		id := subArgs[0]
		if err := client.DiscardFailure(ctx, id); err != nil {
			return err
		}
		formatter.PrintSuccess("Dead-letter failure %s marked discarded.", id)
		return nil

	default:
		return fmt.Errorf("unknown dlq subcommand %q. Available: list, retry, discard", sub)
	}
}
