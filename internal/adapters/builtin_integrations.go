package adapters

// SlackManifest declares the Slack integration manifest.
func SlackManifest() Manifest {
	return Manifest{
		ID:                     "slack",
		Name:                   "Slack",
		Description:            "Send notifications and interactive blocks to Slack channels via incoming webhooks or bot tokens.",
		Category:               "messaging",
		Icon:                   "slack",
		Kind:                   "action",
		Capabilities:           []string{"slack.send_message"},
		RequiredCredentialType: "slack",
		Actions: []ActionMetadata{
			{Type: "send_message", Name: "Send Message", Description: "Post text or blocks to a Slack channel or webhook."},
		},
		Credentials: []CredentialProperty{
			{Key: "webhook_url", Label: "Incoming Webhook URL", Type: "password", Required: false, Description: "Slack incoming webhook URL"},
			{Key: "bot_token", Label: "Bot User OAuth Token", Type: "password", Required: false, Description: "xoxb-... token for chat.postMessage"},
		},
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []any{"text"},
			"properties": map[string]any{
				"webhook_url": map[string]any{"type": "string", "title": "Webhook URL", "description": "Incoming webhook URL (or configured in credential)"},
				"channel":     map[string]any{"type": "string", "title": "Channel ID / Name", "description": "Target channel (e.g. #alerts or C123456)"},
				"text":        map[string]any{"type": "string", "title": "Message Text", "description": "Markdown text to display (supports templating)"},
				"username":    map[string]any{"type": "string", "title": "Bot Display Name"},
				"icon_emoji":  map[string]any{"type": "string", "title": "Emoji Icon", "description": "e.g. :robot_face:"},
			},
		},
		Schemas: map[string]map[string]any{
			"send_message": {
				"type":     "object",
				"required": []any{"text"},
				"properties": map[string]any{
					"webhook_url": map[string]any{"type": "string", "title": "Webhook URL"},
					"channel":     map[string]any{"type": "string", "title": "Channel ID"},
					"text":        map[string]any{"type": "string", "title": "Message Text"},
					"username":    map[string]any{"type": "string", "title": "Bot Display Name"},
					"icon_emoji":  map[string]any{"type": "string", "title": "Emoji Icon"},
				},
			},
		},
	}
}

// TeamsManifest declares the Microsoft Teams integration manifest.
func TeamsManifest() Manifest {
	return Manifest{
		ID:           "teams",
		Name:         "Microsoft Teams",
		Description:  "Deliver structured MessageCards and alerts to Microsoft Teams channels.",
		Category:     "messaging",
		Icon:         "teams",
		Kind:         "action",
		Capabilities: []string{"teams.send_card"},
		Actions: []ActionMetadata{
			{Type: "send_card", Name: "Send Card", Description: "Post an Office 365 Connector card to Teams webhook."},
		},
		Credentials: []CredentialProperty{
			{Key: "webhook_url", Label: "Teams Webhook URL", Type: "password", Required: false, Description: "Channel incoming webhook URL"},
		},
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []any{"text"},
			"properties": map[string]any{
				"webhook_url": map[string]any{"type": "string", "title": "Webhook URL", "description": "Teams connector webhook URL"},
				"title":       map[string]any{"type": "string", "title": "Card Title"},
				"text":        map[string]any{"type": "string", "title": "Card Body Text", "description": "Content of the card"},
				"theme_color": map[string]any{"type": "string", "title": "Theme Color (Hex)", "default": "0076D7"},
				"summary":     map[string]any{"type": "string", "title": "Card Summary"},
			},
		},
		Schemas: map[string]map[string]any{
			"send_card": {
				"type":     "object",
				"required": []any{"text"},
				"properties": map[string]any{
					"webhook_url": map[string]any{"type": "string", "title": "Webhook URL"},
					"title":       map[string]any{"type": "string", "title": "Card Title"},
					"text":        map[string]any{"type": "string", "title": "Card Body Text"},
					"theme_color": map[string]any{"type": "string", "title": "Theme Color (Hex)"},
					"summary":     map[string]any{"type": "string", "title": "Card Summary"},
				},
			},
		},
	}
}

// SMTPManifest declares the SMTP email action integration manifest.
func SMTPManifest() Manifest {
	return Manifest{
		ID:                     "smtp",
		Name:                   "SMTP Email",
		Description:            "Send automated emails via standard SMTP server.",
		Category:               "messaging",
		Icon:                   "mail",
		Kind:                   "action",
		Capabilities:           []string{"smtp.send_email"},
		RequiredCredentialType: "smtp",
		Actions: []ActionMetadata{
			{Type: "send_email", Name: "Send Email", Description: "Send plain text or HTML email."},
		},
		Credentials: []CredentialProperty{
			{Key: "host", Label: "SMTP Host", Type: "text", Required: false, Description: "e.g. smtp.gmail.com"},
			{Key: "username", Label: "Username", Type: "text", Required: false},
			{Key: "password", Label: "Password", Type: "password", Required: false},
		},
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []any{"from", "to", "subject"},
			"properties": map[string]any{
				"host":    map[string]any{"type": "string", "title": "SMTP Host", "default": "smtp.gmail.com"},
				"port":    map[string]any{"type": "integer", "title": "Port", "default": 587},
				"from":    map[string]any{"type": "string", "title": "From Address"},
				"to":      map[string]any{"type": "array", "title": "Recipients", "items": map[string]any{"type": "string"}},
				"subject": map[string]any{"type": "string", "title": "Subject"},
				"body":    map[string]any{"type": "string", "title": "Body"},
				"is_html": map[string]any{"type": "boolean", "title": "Send as HTML", "default": false},
			},
		},
		Schemas: map[string]map[string]any{
			"send_email": {
				"type":     "object",
				"required": []any{"from", "to", "subject"},
				"properties": map[string]any{
					"host":    map[string]any{"type": "string", "title": "SMTP Host"},
					"port":    map[string]any{"type": "integer", "title": "Port", "default": 587},
					"from":    map[string]any{"type": "string", "title": "From Address"},
					"to":      map[string]any{"type": "array", "title": "Recipients", "items": map[string]any{"type": "string"}},
					"subject": map[string]any{"type": "string", "title": "Subject"},
					"body":    map[string]any{"type": "string", "title": "Body"},
					"is_html": map[string]any{"type": "boolean", "title": "Send as HTML"},
				},
			},
		},
	}
}

// CallWorkflowManifest declares the call_workflow chaining integration manifest.
func CallWorkflowManifest() Manifest {
	return Manifest{
		ID:           "call_workflow",
		Name:         "Call Workflow",
		Description:  "Chain execution to another workflow with cycle and recursion depth protection.",
		Category:     "system",
		Icon:         "git-branch",
		Kind:         "action",
		Capabilities: []string{"workflow.call"},
		Actions: []ActionMetadata{
			{Type: "call", Name: "Invoke Workflow", Description: "Trigger a downstream workflow with inherited or custom payload."},
		},
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []any{"workflow_id"},
			"properties": map[string]any{
				"workflow_id": map[string]any{"type": "string", "title": "Target Workflow ID"},
				"payload":     map[string]any{"type": "object", "title": "Custom Child Payload (Optional)"},
				"max_depth":   map[string]any{"type": "integer", "title": "Max Recursion Depth", "default": 5},
			},
		},
		Schemas: map[string]map[string]any{
			"call": {
				"type":     "object",
				"required": []any{"workflow_id"},
				"properties": map[string]any{
					"workflow_id": map[string]any{"type": "string", "title": "Target Workflow ID"},
					"payload":     map[string]any{"type": "object", "title": "Custom Child Payload"},
					"max_depth":   map[string]any{"type": "integer", "title": "Max Recursion Depth", "default": 5},
				},
			},
		},
	}
}

// HTTPPollManifest declares the generic HTTP polling trigger manifest.
func HTTPPollManifest() Manifest {
	return Manifest{
		ID:           "http_poll",
		Name:         "HTTP Poller",
		Description:  "Periodically poll external REST JSON endpoints with ETag / Last-Modified caching and deduplication.",
		Category:     "network",
		Icon:         "refresh-cw",
		Kind:         "trigger",
		Capabilities: []string{"http.poll"},
		Triggers: []TriggerMetadata{
			{Type: "http.poll", Name: "HTTP Poll Item", Description: "Fires when new items appear in polled JSON endpoints."},
		},
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []any{"url"},
			"properties": map[string]any{
				"url":           map[string]any{"type": "string", "title": "Endpoint URL", "description": "JSON API endpoint to poll"},
				"poll_interval": map[string]any{"type": "string", "title": "Polling Interval", "default": "5m"},
				"item_key":      map[string]any{"type": "string", "title": "Unique Item Key Field", "default": "id"},
				"headers":       map[string]any{"type": "object", "title": "HTTP Headers"},
			},
		},
		Schemas: map[string]map[string]any{
			"http.poll": {
				"type":     "object",
				"required": []any{"url"},
				"properties": map[string]any{
					"url":           map[string]any{"type": "string", "title": "Endpoint URL"},
					"poll_interval": map[string]any{"type": "string", "title": "Polling Interval", "default": "5m"},
					"item_key":      map[string]any{"type": "string", "title": "Unique Item Key Field"},
					"headers":       map[string]any{"type": "object", "title": "HTTP Headers"},
				},
			},
		},
	}
}
