package adapters

// IntegrationMetadata describes an integration and its capabilities.
type IntegrationMetadata struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Category    string               `json:"category"` // "messaging", "developer", "feeds", "network"
	Icon        string               `json:"icon"`
	Triggers    []TriggerMetadata    `json:"triggers"`
	Actions     []ActionMetadata     `json:"actions"`
	Credentials []CredentialProperty `json:"credentials"`
}

type TriggerMetadata struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ActionMetadata struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CredentialProperty struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // "text", "password"
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// Catalog returns all available integrations in the Kerob platform.
func Catalog() []IntegrationMetadata {
	return []IntegrationMetadata{
		{
			ID:          "telegram",
			Name:        "Telegram",
			Description: "Monitor channels and send formatted posts, photos, documents, and media groups.",
			Category:    "messaging",
			Icon:        "telegram",
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
		},
		{
			ID:          "discord",
			Name:        "Discord",
			Description: "Publish automated notifications and rich embeds to Discord servers and channels.",
			Category:    "messaging",
			Icon:        "discord",
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
		},
		{
			ID:          "rss",
			Name:        "RSS / Atom Feeds",
			Description: "Poll external RSS or Atom feeds for new articles and items.",
			Category:    "feeds",
			Icon:        "rss",
			Triggers: []TriggerMetadata{
				{Type: "rss.item", Name: "New RSS Item", Description: "Fires whenever a new article or item is published in the feed."},
			},
			Actions:     []ActionMetadata{},
			Credentials: []CredentialProperty{},
		},
		{
			ID:          "github",
			Name:        "GitHub",
			Description: "Receive repository webhooks and automate GitHub issue and comment tracking.",
			Category:    "developer",
			Icon:        "github",
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
		},
		{
			ID:          "webhook",
			Name:        "Inbound Webhooks",
			Description: "Secure inbound HTTP endpoints to ingest events from any external system.",
			Category:    "network",
			Icon:        "webhook",
			Triggers: []TriggerMetadata{
				{Type: "webhook.received", Name: "Inbound Webhook Received", Description: "Fires when an authenticated HTTP POST arrives at /api/v1/webhooks/:id"},
			},
			Actions:     []ActionMetadata{},
			Credentials: []CredentialProperty{},
		},
		{
			ID:          "http",
			Name:        "Generic HTTP API",
			Description: "Execute authenticated HTTP requests (GET, POST, PUT, DELETE) with SSRF defense.",
			Category:    "network",
			Icon:        "globe",
			Triggers:    []TriggerMetadata{},
			Actions: []ActionMetadata{
				{Type: "http_request", Name: "HTTP Request", Description: "Call any external REST API with customizable headers, body, and timeout."},
			},
			Credentials: []CredentialProperty{
				{Key: "api_key", Label: "API Key / Bearer Token", Type: "password", Required: false, Description: "Optional authorization token"},
			},
		},
	}
}
