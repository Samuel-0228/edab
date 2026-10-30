package workflow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"kerob/internal/core/conditions"
	"kerob/internal/core/transformations"
)

// WorkflowTemplate represents a pre-configured automation recipe.
type WorkflowTemplate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Workflow    *Workflow `json:"workflow"`
}

// BuiltinTemplates returns the library of standard templates.
func BuiltinTemplates() []WorkflowTemplate {
	return []WorkflowTemplate{
		{
			ID:          "rss_to_telegram",
			Name:        "RSS Feeds to Telegram Channel",
			Description: "Poll external RSS feeds and mirror new articles directly into a Telegram channel.",
			Category:    "feeds",
			Workflow: &Workflow{
				Name:        "RSS to Telegram Publisher",
				Description: "Automatically broadcast new RSS feed items to Telegram channel",
				Enabled:     true,
				Trigger: TriggerConfig{
					Type:       "rss",
					EventTypes: []string{"rss.item"},
					Config:     json.RawMessage(`{"feed_url":"https://news.ycombinator.com/rss","poll_interval":"5m"}`),
				},
				Transformations: &transformations.Transformation{
					Type:     "template",
					Template: "📢 <b>{{payload.title}}</b>\n\n{{payload.description}}\n\n🔗 {{payload.link}}",
				},
				Actions: []ActionStep{
					{
						ID:          "send_telegram",
						Name:        "Send to Telegram",
						Integration: "telegram",
						ActionType:  "send_message",
						Config:      json.RawMessage(`{"chat_id":"@my_channel","text":"{{transformed.text}}","parse_mode":"HTML"}`),
					},
				},
				MaxRetries:     3,
				TimeoutSeconds: 30,
			},
		},
		{
			ID:          "github_to_slack",
			Name:        "GitHub CI Alerts to Slack",
			Description: "Receive GitHub push or issue notifications and alert engineering channel in Slack.",
			Category:    "developer",
			Workflow: &Workflow{
				Name:        "GitHub Alerts to Slack",
				Description: "Forward critical GitHub events to team Slack",
				Enabled:     true,
				Trigger: TriggerConfig{
					Type:       "github",
					EventTypes: []string{"github.push", "github.issue.opened"},
				},
				Conditions: &conditions.Condition{
					Field:    "payload.action",
					Operator: "equals",
					Value:    "opened",
				},
				Actions: []ActionStep{
					{
						ID:          "notify_slack",
						Name:        "Post Slack Message",
						Integration: "slack",
						ActionType:  "send_message",
						Config:      json.RawMessage(`{"channel":"#dev-alerts","text":"🚨 GitHub Event in {{payload.repository.name}}: {{payload.issue.title}}"}`),
					},
				},
				MaxRetries:     3,
				TimeoutSeconds: 30,
			},
		},
		{
			ID:          "webhook_to_discord",
			Name:        "Inbound Webhooks to Discord Rich Embed",
			Description: "Ingest custom events via authenticated HTTP webhooks and publish rich cards to Discord.",
			Category:    "network",
			Workflow: &Workflow{
				Name:        "Webhook to Discord Card",
				Description: "Deliver rich embeds to Discord from webhook events",
				Enabled:     true,
				Trigger: TriggerConfig{
					Type:       "webhook",
					EventTypes: []string{"webhook.received"},
				},
				Actions: []ActionStep{
					{
						ID:          "discord_embed",
						Name:        "Send Discord Embed",
						Integration: "discord",
						ActionType:  "send_embed",
						Config:      json.RawMessage(`{"title":"New Event Received","description":"{{payload.text}}","color":3447003}`),
					},
				},
				MaxRetries:     3,
				TimeoutSeconds: 30,
			},
		},
		{
			ID:          "schedule_health_check",
			Name:        "Periodic API Health Check with Alerts",
			Description: "Run cron health checks against external HTTP services every 5 minutes.",
			Category:    "system",
			Workflow: &Workflow{
				Name:        "API Health Check Cron",
				Description: "Periodic automated probe of API endpoints",
				Enabled:     true,
				Trigger: TriggerConfig{
					Type:       "schedule",
					EventTypes: []string{"schedule.tick"},
					Config:     json.RawMessage(`{"interval":"5m","cron":"*/5 * * * *"}`),
				},
				Actions: []ActionStep{
					{
						ID:          "check_api",
						Name:        "Call Health Endpoint",
						Integration: "http",
						ActionType:  "http_request",
						Config:      json.RawMessage(`{"method":"GET","url":"https://api.example.com/health"}`),
					},
				},
				MaxRetries:     2,
				TimeoutSeconds: 15,
			},
		},
	}
}

// ExportWorkflowJSON serializes a workflow to indented JSON.
func ExportWorkflowJSON(wf *Workflow) ([]byte, error) {
	return json.MarshalIndent(wf, "", "  ")
}

// ImportWorkflowJSON deserializes a workflow from JSON.
func ImportWorkflowJSON(data []byte) (*Workflow, error) {
	var wf Workflow
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("invalid workflow JSON: %w", err)
	}
	if err := wf.Validate(); err != nil {
		return nil, fmt.Errorf("invalid workflow configuration: %w", err)
	}
	return &wf, nil
}

// ExportWorkflowYAML serializes a workflow to clean standard YAML without third-party dependencies.
func ExportWorkflowYAML(wf *Workflow) ([]byte, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("name: %q\n", wf.Name))
	if wf.Description != "" {
		sb.WriteString(fmt.Sprintf("description: %q\n", wf.Description))
	}
	sb.WriteString(fmt.Sprintf("enabled: %t\n", wf.Enabled))
	sb.WriteString(fmt.Sprintf("max_retries: %d\n", wf.MaxRetries))
	sb.WriteString(fmt.Sprintf("timeout_seconds: %d\n", wf.TimeoutSeconds))
	if wf.RateLimit > 0 {
		sb.WriteString(fmt.Sprintf("rate_limit: %d\n", wf.RateLimit))
	}
	if wf.MaxConcurrency > 0 {
		sb.WriteString(fmt.Sprintf("max_concurrency: %d\n", wf.MaxConcurrency))
	}
	if wf.ConcurrencyKey != "" {
		sb.WriteString(fmt.Sprintf("concurrency_key: %q\n", wf.ConcurrencyKey))
	}

	sb.WriteString("trigger:\n")
	sb.WriteString(fmt.Sprintf("  type: %q\n", wf.Trigger.Type))
	if len(wf.Trigger.EventTypes) > 0 {
		sb.WriteString("  event_types:\n")
		for _, et := range wf.Trigger.EventTypes {
			sb.WriteString(fmt.Sprintf("    - %q\n", et))
		}
	}
	if len(wf.Trigger.Config) > 0 && string(wf.Trigger.Config) != "{}" {
		sb.WriteString(fmt.Sprintf("  config: %s\n", string(wf.Trigger.Config)))
	}

	sb.WriteString("actions:\n")
	for _, act := range wf.Actions {
		sb.WriteString(fmt.Sprintf("  - id: %q\n", act.ID))
		sb.WriteString(fmt.Sprintf("    name: %q\n", act.Name))
		if act.Integration != "" {
			sb.WriteString(fmt.Sprintf("    integration: %q\n", act.Integration))
		}
		if act.ActionType != "" {
			sb.WriteString(fmt.Sprintf("    action_type: %q\n", act.ActionType))
		}
		if len(act.Config) > 0 {
			sb.WriteString(fmt.Sprintf("    config: %s\n", string(act.Config)))
		}
	}

	return []byte(sb.String()), nil
}

// ImportWorkflowYAML deserializes a workflow from YAML.
func ImportWorkflowYAML(data []byte) (*Workflow, error) {
	// If it's valid JSON, handle directly as JSON is a subset of YAML
	var jsonWf Workflow
	if err := json.Unmarshal(data, &jsonWf); err == nil && jsonWf.Name != "" {
		if err := jsonWf.Validate(); err != nil {
			return nil, err
		}
		return &jsonWf, nil
	}

	wf := &Workflow{
		Enabled: true,
	}

	lines := strings.Split(string(data), "\n")
	var currentSection string
	var currentAction *ActionStep

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "trigger:") {
			currentSection = "trigger"
			continue
		}
		if strings.HasPrefix(trimmed, "actions:") {
			currentSection = "actions"
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)

			switch currentSection {
			case "":
				switch k {
				case "id":
					wf.ID = v
				case "name":
					wf.Name = v
				case "description":
					wf.Description = v
				case "enabled":
					wf.Enabled = (v == "true")
				case "max_retries":
					wf.MaxRetries, _ = strconv.Atoi(v)
				case "timeout_seconds":
					wf.TimeoutSeconds, _ = strconv.Atoi(v)
				case "rate_limit":
					wf.RateLimit, _ = strconv.Atoi(v)
				case "max_concurrency":
					wf.MaxConcurrency, _ = strconv.Atoi(v)
				case "concurrency_key":
					wf.ConcurrencyKey = v
				}
			case "trigger":
				switch k {
				case "type":
					wf.Trigger.Type = v
				case "config":
					wf.Trigger.Config = json.RawMessage(v)
				}
			case "actions":
				if strings.HasPrefix(k, "- id") {
					if currentAction != nil {
						wf.Actions = append(wf.Actions, *currentAction)
					}
					currentAction = &ActionStep{ID: v}
				} else if currentAction != nil {
					switch k {
					case "name":
						currentAction.Name = v
					case "integration":
						currentAction.Integration = v
					case "action_type":
						currentAction.ActionType = v
					case "config":
						currentAction.Config = json.RawMessage(v)
					}
				}
			}
		} else if currentSection == "trigger" && strings.HasPrefix(trimmed, "- ") {
			et := strings.Trim(strings.TrimPrefix(trimmed, "- "), `"'`)
			wf.Trigger.EventTypes = append(wf.Trigger.EventTypes, et)
		}
	}

	if currentAction != nil {
		wf.Actions = append(wf.Actions, *currentAction)
	}

	if err := wf.Validate(); err != nil {
		return nil, fmt.Errorf("invalid workflow configuration: %w", err)
	}
	return wf, nil
}
