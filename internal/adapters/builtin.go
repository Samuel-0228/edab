package adapters

// BuiltinIntegrations returns the standard integrations registered in Kerob.
func BuiltinIntegrations() []Integration {
	return []Integration{
		IntegrationFunc(TelegramManifest),
		IntegrationFunc(DiscordManifest),
		IntegrationFunc(RSSManifest),
		IntegrationFunc(GitHubManifest),
		IntegrationFunc(WebhookManifest),
		IntegrationFunc(HTTPManifest),
		IntegrationFunc(ScheduleManifest),
	}
}

// TelegramManifest declares the Telegram integration.
func TelegramManifest() Manifest {
	return Manifest{
		ID:                     "telegram",
		Name:                   "Telegram",
		Description:            "Monitor channels and send formatted posts, photos, documents, and media groups.",
		Category:               "messaging",
		Icon:                   "telegram",
		Kind:                   "both",
		Capabilities:           []string{"telegram.message", "telegram.media_group", "telegram.public_post", "send_message", "copy_message"},
		RequiredCredentialType: "telegram",
		Triggers: []TriggerMetadata{
			{Type: "telegram.message", Name: "New Channel Message", Description: "Fires when a new post arrives in a monitored channel or group."},
			{Type: "telegram.media_group", Name: "New Media Group / Album", Description: "Fires when an album of photos/videos is published."},
			{Type: "telegram.public_post", Name: "New Public Channel Post", Description: "Scraped from public Telegram channels without bot admin access."},
		},
		Actions: []ActionMetadata{
			{Type: "send_message", Name: "Send Message", Description: "Send a formatted text message to a channel or user."},
			{Type: "copy_message", Name: "Copy Message", Description: "Verbatim copy of a message or album preserving original format without forwarded banner."},
		},
		Credentials: []CredentialProperty{
			{Key: "bot_token", Label: "Bot Token", Type: "password", Required: true, Description: "Telegram Bot API token from @BotFather"},
		},
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []any{"chat_id", "text"},
			"properties": map[string]any{
				"chat_id": map[string]any{
					"type":        "string",
					"title":       "Chat / Channel ID",
					"description": "Destination Telegram channel username (@channel) or numeric ID (-100...)",
				},
				"text": map[string]any{
					"type":        "string",
					"title":       "Message Text",
					"description": "Message content to send. Supports template expressions like {{payload.text}}.",
				},
				"parse_mode": map[string]any{
					"type":        "string",
					"title":       "Parse Mode",
					"enum":        []any{"MarkdownV2", "HTML", "Markdown"},
					"default":     "MarkdownV2",
					"description": "Text formatting syntax",
				},
			},
		},
		Schemas: map[string]map[string]any{
			"send_message": {
				"type":     "object",
				"required": []any{"chat_id", "text"},
				"properties": map[string]any{
					"chat_id": map[string]any{"type": "string", "title": "Chat / Channel ID", "description": "Destination Telegram channel username or ID"},
					"text":    map[string]any{"type": "string", "title": "Message Text", "description": "Message content (supports templating)"},
					"parse_mode": map[string]any{
						"type":    "string",
						"title":   "Parse Mode",
						"enum":    []any{"MarkdownV2", "HTML", "Markdown"},
						"default": "MarkdownV2",
					},
				},
			},
			"copy_message": {
				"type":     "object",
				"required": []any{"chat_id"},
				"properties": map[string]any{
					"chat_id":      map[string]any{"type": "string", "title": "Destination Chat ID", "description": "Target chat or channel"},
					"from_chat_id": map[string]any{"type": "string", "title": "Source Chat ID", "description": "Source chat ID of original message"},
					"message_id":   map[string]any{"type": "integer", "title": "Message ID", "description": "Numeric message ID to copy"},
				},
			},
		},
	}
}

// DiscordManifest declares the Discord integration.
func DiscordManifest() Manifest {
	return Manifest{
		ID:                     "discord",
		Name:                   "Discord",
		Description:            "Publish automated notifications and rich embeds to Discord servers and channels.",
		Category:               "messaging",
		Icon:                   "discord",
		Kind:                   "both",
		Capabilities:           []string{"discord.webhook", "send_message", "send_embed"},
		RequiredCredentialType: "discord",
		Triggers: []TriggerMetadata{
			{Type: "discord.webhook", Name: "Incoming Discord Webhook", Description: "Receives events from Discord interactions."},
		},
		Actions: []ActionMetadata{
			{Type: "send_message", Name: "Send Message", Description: "Send a plain or markdown text message to a Discord channel."},
			{Type: "send_embed", Name: "Send Rich Embed", Description: "Publish an embed card with title, description, color, fields, and timestamp."},
		},
		Credentials: []CredentialProperty{
			{Key: "webhook_url", Label: "Discord Webhook URL", Type: "password", Required: false, Description: "Channel webhook URL"},
			{Key: "bot_token", Label: "Bot Token", Type: "password", Required: false, Description: "Discord Bot Token for API access"},
		},
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []any{"content"},
			"properties": map[string]any{
				"webhook_url": map[string]any{"type": "string", "title": "Webhook URL", "description": "Discord channel incoming webhook URL (or configured in credential)"},
				"content":     map[string]any{"type": "string", "title": "Message Content", "description": "Markdown message body (supports templating)"},
				"username":    map[string]any{"type": "string", "title": "Bot Display Name", "description": "Custom name for this message"},
				"avatar_url":  map[string]any{"type": "string", "title": "Avatar URL", "description": "Custom avatar image URL"},
			},
		},
		Schemas: map[string]map[string]any{
			"send_message": {
				"type":     "object",
				"required": []any{"content"},
				"properties": map[string]any{
					"webhook_url": map[string]any{"type": "string", "title": "Webhook URL"},
					"content":     map[string]any{"type": "string", "title": "Message Content"},
					"username":    map[string]any{"type": "string", "title": "Bot Display Name"},
					"avatar_url":  map[string]any{"type": "string", "title": "Avatar URL"},
				},
			},
			"send_embed": {
				"type":     "object",
				"required": []any{"title"},
				"properties": map[string]any{
					"webhook_url": map[string]any{"type": "string", "title": "Webhook URL"},
					"title":       map[string]any{"type": "string", "title": "Embed Title"},
					"description": map[string]any{"type": "string", "title": "Embed Description"},
					"color":       map[string]any{"type": "integer", "title": "Embed Color (Decimal)"},
					"author_name": map[string]any{"type": "string", "title": "Author Name"},
					"footer_text": map[string]any{"type": "string", "title": "Footer Text"},
				},
			},
		},
	}
}

// RSSManifest declares the RSS/Atom feeds integration.
func RSSManifest() Manifest {
	return Manifest{
		ID:           "rss",
		Name:         "RSS / Atom Feeds",
		Description:  "Poll external RSS or Atom feeds for new articles and items.",
		Category:     "feeds",
		Icon:         "rss",
		Kind:         "trigger",
		Capabilities: []string{"rss.item"},
		Triggers: []TriggerMetadata{
			{Type: "rss.item", Name: "New RSS Item", Description: "Fires whenever a new article or item is published in the feed."},
		},
		Actions:     []ActionMetadata{},
		Credentials: []CredentialProperty{},
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []any{"feed_url"},
			"properties": map[string]any{
				"feed_url":      map[string]any{"type": "string", "title": "Feed URL", "description": "Public RSS or Atom feed endpoint"},
				"poll_interval": map[string]any{"type": "string", "title": "Poll Interval", "default": "5m", "description": "Polling cadence (e.g. 1m, 5m, 1h)"},
			},
		},
		Schemas: map[string]map[string]any{
			"rss.item": {
				"type":     "object",
				"required": []any{"feed_url"},
				"properties": map[string]any{
					"feed_url":      map[string]any{"type": "string", "title": "Feed URL"},
					"poll_interval": map[string]any{"type": "string", "title": "Poll Interval", "default": "5m"},
				},
			},
		},
	}
}

// GitHubManifest declares the GitHub integration.
func GitHubManifest() Manifest {
	return Manifest{
		ID:                     "github",
		Name:                   "GitHub",
		Description:            "Receive repository webhooks and automate GitHub issue and comment tracking.",
		Category:               "developer",
		Icon:                   "github",
		Kind:                   "both",
		Capabilities:           []string{"github.issue.opened", "github.push", "github.pull_request", "create_issue"},
		RequiredCredentialType: "github",
		Triggers: []TriggerMetadata{
			{Type: "github.issue.opened", Name: "Issue Opened", Description: "Fires when an issue is opened in the repository."},
			{Type: "github.push", Name: "Git Push", Description: "Fires on branch or tag push."},
			{Type: "github.pull_request", Name: "Pull Request Event", Description: "Fires on PR open, close, or review."},
		},
		Actions: []ActionMetadata{
			{Type: "create_issue", Name: "Create Issue", Description: "Open a new issue in a GitHub repository."},
		},
		Credentials: []CredentialProperty{
			{Key: "personal_access_token", Label: "GitHub Token", Type: "password", Required: false, Description: "Personal Access Token (classic or fine-grained)"},
			{Key: "webhook_secret", Label: "Webhook Secret", Type: "password", Required: false, Description: "Secret configured in GitHub repository webhook settings"},
		},
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []any{"owner", "repo", "title"},
			"properties": map[string]any{
				"owner":  map[string]any{"type": "string", "title": "Repository Owner", "description": "GitHub user or organization"},
				"repo":   map[string]any{"type": "string", "title": "Repository Name", "description": "Repository identifier"},
				"title":  map[string]any{"type": "string", "title": "Issue Title", "description": "Title of the issue (supports templating)"},
				"body":   map[string]any{"type": "string", "title": "Issue Body", "description": "Issue markdown description"},
				"labels": map[string]any{"type": "array", "title": "Labels", "items": map[string]any{"type": "string"}},
			},
		},
		Schemas: map[string]map[string]any{
			"create_issue": {
				"type":     "object",
				"required": []any{"owner", "repo", "title"},
				"properties": map[string]any{
					"owner":  map[string]any{"type": "string", "title": "Repository Owner"},
					"repo":   map[string]any{"type": "string", "title": "Repository Name"},
					"title":  map[string]any{"type": "string", "title": "Issue Title"},
					"body":   map[string]any{"type": "string", "title": "Issue Body"},
					"labels": map[string]any{"type": "array", "title": "Labels", "items": map[string]any{"type": "string"}},
				},
			},
		},
	}
}

// WebhookManifest declares the generic inbound webhook integration.
func WebhookManifest() Manifest {
	return Manifest{
		ID:           "webhook",
		Name:         "Inbound Webhooks",
		Description:  "Secure inbound HTTP endpoints to ingest events from any external system.",
		Category:     "network",
		Icon:         "webhook",
		Kind:         "trigger",
		Capabilities: []string{"webhook.received"},
		Triggers: []TriggerMetadata{
			{Type: "webhook.received", Name: "Inbound Webhook Received", Description: "Fires when an authenticated HTTP POST arrives at /api/v1/webhooks/:id"},
		},
		Actions:     []ActionMetadata{},
		Credentials: []CredentialProperty{},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "title": "Custom Endpoint Path"},
				"secret": map[string]any{"type": "string", "title": "Secret Token", "description": "Authorization bearer token or secret"},
			},
		},
		Schemas: map[string]map[string]any{
			"webhook.received": {
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "title": "Custom Endpoint Path"},
					"secret": map[string]any{"type": "string", "title": "Secret Token"},
				},
			},
		},
	}
}

// HTTPManifest declares the generic outbound HTTP API action integration.
func HTTPManifest() Manifest {
	return Manifest{
		ID:                     "http",
		Name:                   "Generic HTTP API",
		Description:            "Execute authenticated HTTP requests (GET, POST, PUT, DELETE) with SSRF defense.",
		Category:               "network",
		Icon:                   "globe",
		Kind:                   "action",
		Capabilities:           []string{"http_request"},
		RequiredCredentialType: "http",
		Triggers:               []TriggerMetadata{},
		Actions: []ActionMetadata{
			{Type: "http_request", Name: "HTTP Request", Description: "Call any external REST API with customizable headers, body, and timeout."},
		},
		Credentials: []CredentialProperty{
			{Key: "api_key", Label: "API Key / Bearer Token", Type: "password", Required: false, Description: "Optional authorization token"},
		},
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []any{"url", "method"},
			"properties": map[string]any{
				"url":     map[string]any{"type": "string", "title": "Target URL", "description": "Destination endpoint (SSRF protected)"},
				"method":  map[string]any{"type": "string", "title": "HTTP Method", "enum": []any{"GET", "POST", "PUT", "PATCH", "DELETE"}, "default": "POST"},
				"headers": map[string]any{"type": "object", "title": "Headers", "description": "Key-value map of HTTP headers"},
				"body":    map[string]any{"type": "string", "title": "Request Body", "description": "Raw payload string or JSON (supports templating)"},
			},
		},
		Schemas: map[string]map[string]any{
			"http_request": {
				"type":     "object",
				"required": []any{"url", "method"},
				"properties": map[string]any{
					"url":     map[string]any{"type": "string", "title": "Target URL"},
					"method":  map[string]any{"type": "string", "title": "HTTP Method", "enum": []any{"GET", "POST", "PUT", "PATCH", "DELETE"}, "default": "POST"},
					"headers": map[string]any{"type": "object", "title": "Headers"},
					"body":    map[string]any{"type": "string", "title": "Request Body"},
				},
			},
		},
	}
}

// ScheduleManifest declares the cron / interval scheduler trigger integration.
func ScheduleManifest() Manifest {
	return Manifest{
		ID:           "schedule",
		Name:         "Schedule",
		Description:  "Periodically trigger workflows via cron expressions or fixed time intervals.",
		Category:     "system",
		Icon:         "clock",
		Kind:         "trigger",
		Capabilities: []string{"schedule.cron", "schedule.interval"},
		Triggers: []TriggerMetadata{
			{Type: "schedule.tick", Name: "Schedule Tick", Description: "Fires on scheduled cron intervals or fixed recurring durations."},
		},
		Actions:     []ActionMetadata{},
		Credentials: []CredentialProperty{},
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"interval": map[string]any{"type": "string", "title": "Interval Duration", "description": "Fixed time interval (e.g. 1m, 5m, 1h, 24h)"},
				"cron":     map[string]any{"type": "string", "title": "Cron Expression", "description": "Standard 5-field cron expression, e.g. */5 * * * *"},
			},
		},
		Schemas: map[string]map[string]any{
			"schedule.tick": {
				"type": "object",
				"properties": map[string]any{
					"interval": map[string]any{"type": "string", "title": "Interval Duration"},
					"cron":     map[string]any{"type": "string", "title": "Cron Expression"},
				},
			},
		},
	}
}
