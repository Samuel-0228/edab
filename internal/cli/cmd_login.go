package cli

import (
	"context"
	"flag"
	"fmt"

	"kerob/pkg/sdk"
)

func runLogin(ctx context.Context, cfg *Config, formatter *OutputFormatter, args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(formatter.ErrOut)

	endpoint := fs.String("endpoint", cfg.Endpoint, "Kerob server endpoint URL")
	token := fs.String("token", "", "API token or Service Account token")
	tenant := fs.String("tenant", cfg.TenantID, "Default tenant identifier")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *token == "" && len(fs.Args()) > 0 {
		*token = fs.Args()[0]
	}

	if *endpoint != "" {
		cfg.Endpoint = *endpoint
	}
	if *token != "" {
		cfg.Token = *token
	}
	if *tenant != "" {
		cfg.TenantID = *tenant
	}

	// Verify credentials if token provided
	if cfg.Token != "" {
		client, err := sdk.NewClient(cfg.Endpoint, sdk.WithToken(cfg.Token), sdk.WithTenant(cfg.TenantID))
		if err != nil {
			return err
		}
		me, err := client.GetMe(ctx)
		if err != nil {
			formatter.PrintError("Authentication verification failed: %s", err.Error())
			return err
		}
		if me.TenantID != "" && cfg.TenantID == "" {
			cfg.TenantID = me.TenantID
		}
		formatter.PrintSuccess("Authenticated as %s (%s) [Role: %s, Tenant: %s]", me.Name, me.Email, me.Role, me.TenantName)
	}

	if err := SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	formatter.PrintSuccess("Configuration saved to %s", ConfigPath())
	return nil
}

func runTenant(ctx context.Context, client *sdk.Client, cfg *Config, formatter *OutputFormatter, args []string) error {
	if len(args) == 0 {
		return runTenantCurrent(cfg, formatter)
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list":
		tenants, err := client.ListTenants(ctx)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(tenants)
		}
		headers := []string{"TENANT ID", "NAME", "SLUG", "CURRENT"}
		var rows [][]string
		for _, t := range tenants {
			isCurrent := ""
			if t.ID == cfg.TenantID {
				isCurrent = "*"
			}
			rows = append(rows, []string{t.ID, t.Name, t.Slug, isCurrent})
		}
		formatter.PrintTable(headers, rows)
		return nil

	case "select":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: kerob tenant select <tenant_id>")
		}
		targetID := subArgs[0]
		// Verify tenant exists
		t, err := client.GetTenant(ctx, targetID)
		if err != nil {
			return fmt.Errorf("failed to verify tenant %q: %w", targetID, err)
		}
		cfg.TenantID = t.ID
		if err := SaveConfig(cfg); err != nil {
			return err
		}
		formatter.PrintSuccess("Active tenant switched to %s (%s)", t.Name, t.ID)
		return nil

	case "current":
		return runTenantCurrent(cfg, formatter)

	default:
		return fmt.Errorf("unknown tenant subcommand %q. Available: list, select, current", sub)
	}
}

func runTenantCurrent(cfg *Config, formatter *OutputFormatter) error {
	if cfg.TenantID == "" {
		if formatter.JSONMode {
			return formatter.PrintJSON(map[string]any{"tenant_id": nil, "message": "no tenant selected (single-tenant or global mode)"})
		}
		formatter.PrintError("No active tenant selected. Use 'kerob tenant select <id>' or set KEROB_TENANT_ID.")
		return nil
	}
	if formatter.JSONMode {
		return formatter.PrintJSON(map[string]string{"tenant_id": cfg.TenantID})
	}
	_, _ = fmt.Fprintf(formatter.Out, "Current tenant: %s\n", cfg.TenantID)
	return nil
}
