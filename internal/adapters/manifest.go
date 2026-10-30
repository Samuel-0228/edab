package adapters

// Manifest describes an integration, its capabilities, and its JSON Schema.
type Manifest struct {
	ID                     string                    `json:"id"`
	Name                   string                    `json:"name"`
	Description            string                    `json:"description"`
	Category               string                    `json:"category"` // "messaging", "developer", "feeds", "network", "system"
	Icon                   string                    `json:"icon"`
	Kind                   string                    `json:"kind"` // "trigger", "action", "both"
	Capabilities           []string                  `json:"capabilities"`
	RequiredCredentialType string                    `json:"required_credential_type,omitempty"`
	ConfigSchema           map[string]any            `json:"config_schema,omitempty"`
	Schemas                map[string]map[string]any `json:"schemas,omitempty"`
	Triggers               []TriggerMetadata         `json:"triggers"`
	Actions                []ActionMetadata          `json:"actions"`
	Credentials            []CredentialProperty      `json:"credentials"`
}

// IntegrationMetadata is a backward-compatible type alias for Manifest.
type IntegrationMetadata = Manifest

// TriggerMetadata describes a trigger event emitted or received by an integration.
type TriggerMetadata struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ActionMetadata describes an action operation provided by an integration.
type ActionMetadata struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CredentialProperty defines a required or optional credential secret.
type CredentialProperty struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // "text", "password"
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// Integration defines the interface required by the Kerob Integration SDK.
type Integration interface {
	Manifest() Manifest
}

// IntegrationFunc allows simple functions to implement the Integration interface.
type IntegrationFunc func() Manifest

// Manifest implements Integration.
func (f IntegrationFunc) Manifest() Manifest {
	return f()
}
