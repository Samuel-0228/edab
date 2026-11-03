package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"kerob/pkg/sdk"
)

func runGovernance(ctx context.Context, client *sdk.Client, formatter *OutputFormatter, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("subcommand required (get, verify, prune, export)")
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "get":
		policy, err := client.GetGovernancePolicy(ctx)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(policy)
		}
		headers := []string{"Tenant", "Mode", "Retries", "Retention", "Connectors", "Domains"}
		connectors := strings.Join(policy.AllowedConnectors, ",")
		if connectors == "" {
			connectors = "*"
		}
		domains := strings.Join(policy.AllowedOutboundDomains, ",")
		if domains == "" {
			domains = "*"
		}
		rows := [][]string{{
			policy.TenantID,
			policy.EnforcementMode,
			strconv.Itoa(policy.MaxWorkflowRetries),
			fmt.Sprintf("%dd", policy.DataRetentionDays),
			connectors,
			domains,
		}}
		formatter.PrintTable(headers, rows)
		return nil

	case "verify":
		res, err := client.VerifyAuditChain(ctx)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(res)
		}
		if res.Valid {
			formatter.PrintSuccess("Audit chain is valid. %d cryptographically chained entries verified for tenant %s.", res.TotalRecords, res.TenantID)
			return nil
		}
		return fmt.Errorf("audit chain integrity broken: %s", res.ErrorMessage)

	case "prune":
		res, err := client.PruneRetention(ctx)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(res)
		}
		formatter.PrintSuccess("Pruned %d audit logs exceeding %d days retention for tenant %s.", res.PrunedAuditLogs, res.RetentionDays, res.TenantID)
		return nil

	case "export":
		format := "json"
		if len(subArgs) > 0 && (subArgs[0] == "csv" || subArgs[0] == "json") {
			format = subArgs[0]
		}
		data, err := client.ExportAuditLogs(ctx, format, nil)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(formatter.Out, string(data))
		return nil

	default:
		return fmt.Errorf("unknown policy subcommand %q", sub)
	}
}
