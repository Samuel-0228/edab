package cli

import (
	"context"
	"flag"
	"fmt"
	"time"

	"kerob/pkg/sdk"
)

func runEvent(ctx context.Context, client *sdk.Client, formatter *OutputFormatter, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing event subcommand. Available: list, get, replay")
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list":
		fs := flag.NewFlagSet("event list", flag.ContinueOnError)
		fs.SetOutput(formatter.ErrOut)
		limit := fs.Int("limit", 50, "Limit number of records returned")
		offset := fs.Int("offset", 0, "Offset number of records")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}

		events, err := client.ListEvents(ctx, *limit, *offset)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(events)
		}
		headers := []string{"EVENT ID", "SOURCE ID", "EVENT TYPE", "INGESTED AT"}
		var rows [][]string
		for _, ev := range events {
			rows = append(rows, []string{
				ev.ID,
				ev.SourceID,
				ev.EventType,
				ev.CreatedAt.Format(time.RFC3339),
			})
		}
		formatter.PrintTable(headers, rows)
		return nil

	case "get":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob event get <id>")
		}
		ev, err := client.GetEvent(ctx, subArgs[0])
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(ev)
		}
		_, _ = fmt.Fprintf(formatter.Out, "Event ID:    %s\n", ev.ID)
		_, _ = fmt.Fprintf(formatter.Out, "Source ID:   %s\n", ev.SourceID)
		_, _ = fmt.Fprintf(formatter.Out, "Type:        %s\n", ev.EventType)
		_, _ = fmt.Fprintf(formatter.Out, "Ingested At: %s\n", ev.CreatedAt.Format(time.RFC3339))
		_, _ = fmt.Fprintf(formatter.Out, "Payload:\n%s\n", string(ev.Payload))
		return nil

	case "replay":
		fs := flag.NewFlagSet("event replay", flag.ContinueOnError)
		fs.SetOutput(formatter.ErrOut)
		bypassIdempotency := fs.Bool("bypass-idempotency", false, "Bypass duplicate event deduplication constraints")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		if len(fs.Args()) == 0 {
			return fmt.Errorf("usage: kerob event replay <event_id> [--bypass-idempotency]")
		}
		eventID := fs.Args()[0]
		res, err := client.ReplayEvent(ctx, eventID, *bypassIdempotency)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(res)
		}
		formatter.PrintSuccess("Event %s queued for automation replay (bypass_idempotency: %t).", eventID, *bypassIdempotency)
		return nil

	default:
		return fmt.Errorf("unknown event subcommand %q. Available: list, get, replay", sub)
	}
}
