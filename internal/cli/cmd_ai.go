package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"kerob/pkg/sdk"
)

func runAI(ctx context.Context, client *sdk.Client, formatter *OutputFormatter, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("subcommand required (status, draft, explain, proposals, accept, reject)")
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "status":
		status, err := client.GetAIStatus(ctx)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(status)
		}
		headers := []string{"Enabled", "Provider", "Model", "Tenant", "Opted Out", "Max Tokens", "Timeout"}
		rows := [][]string{{
			strconv.FormatBool(status.Enabled),
			status.Provider,
			status.Model,
			status.TenantID,
			strconv.FormatBool(status.OptedOut),
			strconv.Itoa(status.MaxTokens),
			fmt.Sprintf("%ds", status.TimeoutSeconds),
		}}
		formatter.PrintTable(headers, rows)
		return nil

	case "draft":
		if len(subArgs) == 0 {
			return fmt.Errorf("prompt required: kerob ai draft \"<instructions>\" [--connectors http,telegram]")
		}
		prompt := subArgs[0]
		var connectors []string
		for i := 1; i < len(subArgs); i++ {
			if subArgs[i] == "--connectors" && i+1 < len(subArgs) {
				connectors = strings.Split(subArgs[i+1], ",")
				i++
			}
		}

		proposal, err := client.DraftWorkflowAI(ctx, sdk.AIDraftWorkflowRequest{
			Prompt:            prompt,
			AllowedConnectors: connectors,
		})
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(proposal)
		}
		formatter.PrintSuccess("Drafted workflow proposal %s (Status: %s)", proposal.ID, proposal.Status)
		formatter.PrintInfo("Explanation: %s", proposal.Explanation)
		if !proposal.Validation.Valid {
			formatter.PrintWarning("Validation errors: %s", strings.Join(proposal.Validation.Errors, "; "))
		}
		formatter.PrintInfo("To accept: kerob ai accept %s --create-workflow", proposal.ID)
		return nil

	case "explain":
		if len(subArgs) == 0 {
			return fmt.Errorf("execution_id required: kerob ai explain <execution_id>")
		}
		execID := subArgs[0]
		exec, err := client.GetExecution(ctx, execID)
		if err != nil {
			return fmt.Errorf("failed to fetch execution: %w", err)
		}

		errMsg := "Unknown error"
		if exec.Error != "" {
			errMsg = exec.Error
		}

		proposal, err := client.ExplainFailureAI(ctx, sdk.AIExplainFailureRequest{
			WorkflowID:  exec.WorkflowID,
			ExecutionID: exec.ID,
			Error:       errMsg,
		})
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(proposal)
		}
		formatter.PrintSuccess("Failure Diagnosis Proposal %s", proposal.ID)
		formatter.PrintInfo("Explanation: %s", proposal.Explanation)
		if len(proposal.Content) > 0 {
			formatter.PrintInfo("Remediation Content: %s", string(proposal.Content))
		}
		return nil

	case "proposals", "list":
		limit := 20
		list, err := client.ListAIProposals(ctx, "", "", limit, 0)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(list)
		}
		if len(list) == 0 {
			formatter.PrintInfo("No AI proposals found.")
			return nil
		}
		headers := []string{"ID", "Task", "Status", "Valid", "Tokens", "Created"}
		var rows [][]string
		for _, p := range list {
			rows = append(rows, []string{
				p.ID,
				string(p.Task),
				string(p.Status),
				strconv.FormatBool(p.Validation.Valid),
				strconv.Itoa(p.TokensUsed),
				p.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
		formatter.PrintTable(headers, rows)
		return nil

	case "accept":
		if len(subArgs) == 0 {
			return fmt.Errorf("proposal ID required: kerob ai accept <proposal_id> [--create-workflow]")
		}
		propID := subArgs[0]
		createWf := false
		for _, arg := range subArgs[1:] {
			if arg == "--create-workflow" {
				createWf = true
			}
		}

		res, err := client.AcceptAIProposal(ctx, propID, createWf)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(res)
		}
		formatter.PrintSuccess("Proposal %s accepted.", propID)
		if res.AppliedWorkflowID != "" {
			formatter.PrintSuccess("Workflow instantiated with ID: %s", res.AppliedWorkflowID)
		}
		return nil

	case "reject":
		if len(subArgs) == 0 {
			return fmt.Errorf("proposal ID required: kerob ai reject <proposal_id> [--reason \"<note>\"]")
		}
		propID := subArgs[0]
		reason := ""
		for i := 1; i < len(subArgs); i++ {
			if subArgs[i] == "--reason" && i+1 < len(subArgs) {
				reason = subArgs[i+1]
				i++
			}
		}

		res, err := client.RejectAIProposal(ctx, propID, reason)
		if err != nil {
			return err
		}
		if formatter.JSONMode {
			return formatter.PrintJSON(res)
		}
		formatter.PrintSuccess("Proposal %s rejected.", propID)
		return nil

	default:
		return fmt.Errorf("unknown ai subcommand %q (valid: status, draft, explain, proposals, accept, reject)", sub)
	}
}
