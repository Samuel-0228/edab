package connector

import (
	"context"
	"encoding/json"
	"fmt"

	"kerob/internal/adapters"
	"kerob/internal/core/workflow"
)

// ToAdapterManifest converts a connector SDK Manifest to an internal adapters.Manifest.
func ToAdapterManifest(c Connector) adapters.Manifest {
	m := c.Manifest()

	// Determine kind
	hasTriggers := len(m.Triggers) > 0 || m.HasCapability(CapabilityTrigger) || m.HasCapability(CapabilityPolling)
	hasActions := len(m.Actions) > 0 || m.HasCapability(CapabilityAction)
	kind := "action"
	if hasTriggers && hasActions {
		kind = "both"
	} else if hasTriggers {
		kind = "trigger"
	}

	triggers := make([]adapters.TriggerMetadata, len(m.Triggers))
	for i, t := range m.Triggers {
		triggers[i] = adapters.TriggerMetadata{
			Type:        t.Type,
			Name:        t.Name,
			Description: t.Description,
		}
	}

	actions := make([]adapters.ActionMetadata, len(m.Actions))
	for i, a := range m.Actions {
		actions[i] = adapters.ActionMetadata{
			Type:        a.Type,
			Name:        a.Name,
			Description: a.Description,
		}
	}

	creds := make([]adapters.CredentialProperty, len(m.Credentials))
	for i, cp := range m.Credentials {
		creds[i] = adapters.CredentialProperty{
			Key:         cp.Key,
			Label:       cp.Label,
			Type:        cp.Type,
			Required:    cp.Required,
			Description: cp.Description,
		}
	}

	reqCred := m.RequiredCredentialType
	if reqCred == "" && len(creds) > 0 {
		reqCred = m.ID
	}

	return adapters.Manifest{
		ID:                     m.ID,
		Name:                   m.Name,
		Description:            m.Description,
		Category:               m.Category,
		Icon:                   m.Icon,
		Kind:                   kind,
		Capabilities:           m.Capabilities,
		RequiredCredentialType: reqCred,
		ConfigSchema:           m.ConfigSchema,
		Schemas:                m.Schemas,
		Triggers:               triggers,
		Actions:                actions,
		Credentials:            creds,
	}
}

// ActionBridge adapts a Connector that implements ActionProvider to workflow.ActionExecutor.
type ActionBridge struct {
	connector Connector
	provider  ActionProvider
}

// NewActionBridge creates an ActionBridge for a connector.
func NewActionBridge(c Connector) (*ActionBridge, error) {
	provider, ok := c.(ActionProvider)
	if !ok {
		return nil, fmt.Errorf("connector %q does not implement ActionProvider", c.Manifest().ID)
	}
	return &ActionBridge{
		connector: c,
		provider:  provider,
	}, nil
}

func (b *ActionBridge) Name() string {
	return b.connector.Manifest().Name
}

func (b *ActionBridge) Type() string {
	return b.connector.Manifest().ID
}

func (b *ActionBridge) Validate(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}
	var cfgMap map[string]any
	if err := json.Unmarshal(config, &cfgMap); err != nil {
		return fmt.Errorf("malformed action config JSON: %w", err)
	}
	return b.connector.ValidateConfig(cfgMap)
}

func (b *ActionBridge) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	connReq := ActionRequest{
		ExecutionID: req.WorkflowID,
		StepID:      req.ActionID,
		ActionType:  req.ActionID,
		Config:      req.Config,
		Parameters:  req.Payload,
		Credentials: req.Credentials,
	}
	if req.Event != nil && len(req.Event.Payload) > 0 {
		var eventPayload map[string]any
		if err := json.Unmarshal(req.Event.Payload, &eventPayload); err == nil {
			connReq.Event = eventPayload
		}
	}

	res, err := b.provider.ExecuteAction(ctx, connReq)
	if err != nil {
		return workflow.ActionResult{
			Success: false,
		}, err
	}

	var outputRaw json.RawMessage
	if res.Output != nil {
		if bytes, err := json.Marshal(res.Output); err == nil {
			outputRaw = bytes
		}
	}

	metadata := make(map[string]any)
	if res.Error != "" {
		metadata["error"] = res.Error
	}
	if res.Retryable {
		metadata["retryable"] = true
	}

	return workflow.ActionResult{
		Success:  res.Success,
		Output:   outputRaw,
		Metadata: metadata,
	}, nil
}

// IntegrationBridge wraps a Connector to satisfy adapters.Integration.
type IntegrationBridge struct {
	manifest adapters.Manifest
}

func (ib *IntegrationBridge) Manifest() adapters.Manifest {
	return ib.manifest
}

// BridgeToSystem registers a connector with both the adapters.Registry and workflow.ActionRegistry.
func BridgeToSystem(c Connector, adapterReg *adapters.Registry, actionReg *workflow.ActionRegistry) error {
	if c == nil {
		return fmt.Errorf("connector is nil")
	}

	// Register in adapter catalog
	if adapterReg != nil {
		manifest := ToAdapterManifest(c)
		adapterReg.Register(&IntegrationBridge{manifest: manifest})
	}

	// If it provides actions, register in workflow engine action registry
	if actionReg != nil {
		if _, ok := c.(ActionProvider); ok {
			bridge, err := NewActionBridge(c)
			if err != nil {
				return err
			}
			actionReg.Register(bridge)
		}
	}

	return nil
}
