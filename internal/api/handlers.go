package api

import (
	"encoding/json"
	"net/http"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/auth"
	"kerob/internal/core/credentials"
	"kerob/internal/core/delivery"
	"kerob/internal/core/errtracker"
	"kerob/internal/core/event"
	"kerob/internal/core/governance"
	"kerob/internal/core/oauth"
	"kerob/internal/core/routing"
	"kerob/internal/core/schema"
	"kerob/internal/core/streaming"
	"kerob/internal/core/webhooks"
	"kerob/internal/core/workflow"
	"kerob/internal/telegram"
	"kerob/internal/version"
	"kerob/pkg/connector"
)

// Handlers houses the HTTP handler implementations across all Kerob domains.
type Handlers struct {
	// Workflow engine domain
	workflows      workflow.Store
	executions     workflow.ExecutionStore
	deadLetters    workflow.DeadLetterStore
	workflowEngine *workflow.Engine
	actionReg      *workflow.ActionRegistry
	creds          credentials.Store
	webhookStore   webhooks.Store
	versionStore   workflow.VersionStore

	// Event & adapter routing
	sources      sources.Store
	destinations destinations.Store
	routes       routing.Store
	events       event.Store
	deliveries   delivery.Store
	pipeline     *workflow.Pipeline
	sourceReg    *sources.Registry
	destReg      *destinations.Registry
	telegramAPI  telegram.API
	errorTracker errtracker.Provider
	authStore    auth.Store
	multiTenancy bool
	liveHub      *LiveHub
	connectorReg *connector.Registry
	oauthManager *oauth.Manager

	// Event Streaming & Schema Registry
	schemaReg      *schema.Registry
	streamAdapter  streaming.StreamAdapter
	streamOffsets  streaming.OffsetStore
	streamDLQStore streaming.DeadLetterStore
	streamReplay   *streaming.ReplayEngine

	// Sagas & Human Approvals
	approvals workflow.ApprovalStore

	// Governance & Policy Engine
	governanceEngine *governance.GovernanceEngine
}

// Config bundles all dependencies for NewHandlers.
type Config struct {
	Workflows      workflow.Store
	Executions     workflow.ExecutionStore
	DeadLetters    workflow.DeadLetterStore
	WorkflowEngine *workflow.Engine
	ActionReg      *workflow.ActionRegistry
	Creds          credentials.Store
	WebhookStore   webhooks.Store
	VersionStore   workflow.VersionStore
	AuthStore      auth.Store
	MultiTenancy   bool
	LiveHub        *LiveHub
	ConnectorReg   *connector.Registry
	OAuthManager   *oauth.Manager

	SchemaRegistry *schema.Registry
	StreamAdapter  streaming.StreamAdapter
	StreamOffsets  streaming.OffsetStore
	StreamDLQStore streaming.DeadLetterStore
	StreamReplay   *streaming.ReplayEngine

	Approvals workflow.ApprovalStore

	GovernanceEngine *governance.GovernanceEngine

	Sources      sources.Store
	Destinations destinations.Store
	Routes       routing.Store
	Events       event.Store
	Deliveries   delivery.Store
	Pipeline     *workflow.Pipeline
	SourceReg    *sources.Registry
	DestReg      *destinations.Registry
	TelegramAPI  telegram.API
	ErrorTracker errtracker.Provider
}

func NewHandlersFromConfig(cfg Config) *Handlers {
	vStore := cfg.VersionStore
	if vStore == nil {
		vStore = workflow.NewMemoryVersionStore()
	}
	hub := cfg.LiveHub
	if hub == nil {
		hub = NewLiveHub()
	}
	if cfg.WorkflowEngine != nil {
		cfg.WorkflowEngine.SetEventListener(hub)
	}
	cReg := cfg.ConnectorReg
	if cReg == nil {
		cReg = connector.Default()
	}
	oMgr := cfg.OAuthManager
	if oMgr == nil && cfg.Creds != nil {
		oMgr = oauth.NewManager(cReg, cfg.Creds, nil)
	}

	scReg := cfg.SchemaRegistry
	if scReg == nil {
		scReg = schema.NewRegistry(schema.NewMemoryStore())
	}
	strAdapter := cfg.StreamAdapter
	if strAdapter == nil {
		strAdapter = streaming.NewMemoryStreamAdapter(4)
	}
	offStore := cfg.StreamOffsets
	if offStore == nil {
		offStore = streaming.NewMemoryOffsetStore()
	}
	dlqStore := cfg.StreamDLQStore
	if dlqStore == nil {
		dlqStore = streaming.NewMemoryDeadLetterStore()
	}
	repEngine := cfg.StreamReplay
	if repEngine == nil {
		producer, _ := strAdapter.Producer()
		repEngine = streaming.NewReplayEngine(cfg.Events, cfg.WorkflowEngine, dlqStore, producer)
	}

	appStore := cfg.Approvals
	if appStore == nil && cfg.WorkflowEngine != nil && cfg.WorkflowEngine.Approvals() != nil {
		appStore = cfg.WorkflowEngine.Approvals()
	}
	if appStore == nil {
		appStore = workflow.NewMemoryApprovalStore()
	}

	govEngine := cfg.GovernanceEngine
	if govEngine == nil {
		govEngine = governance.NewGovernanceEngine(nil)
	}
	if cfg.WorkflowEngine != nil && cfg.WorkflowEngine.Governance() == nil {
		cfg.WorkflowEngine.SetGovernance(govEngine)
	}

	return &Handlers{
		workflows:      cfg.Workflows,
		executions:     cfg.Executions,
		deadLetters:    cfg.DeadLetters,
		workflowEngine: cfg.WorkflowEngine,
		actionReg:      cfg.ActionReg,
		creds:          cfg.Creds,
		webhookStore:   cfg.WebhookStore,
		versionStore:   vStore,

		sources:      cfg.Sources,
		destinations: cfg.Destinations,
		routes:       cfg.Routes,
		events:       cfg.Events,
		deliveries:   cfg.Deliveries,
		pipeline:     cfg.Pipeline,
		sourceReg:    cfg.SourceReg,
		destReg:      cfg.DestReg,
		telegramAPI:  cfg.TelegramAPI,
		errorTracker: cfg.ErrorTracker,
		authStore:    cfg.AuthStore,
		multiTenancy: cfg.MultiTenancy,
		liveHub:      hub,
		connectorReg: cReg,
		oauthManager: oMgr,

		schemaReg:      scReg,
		streamAdapter:  strAdapter,
		streamOffsets:  offStore,
		streamDLQStore: dlqStore,
		streamReplay:   repEngine,

		approvals:        appStore,
		governanceEngine: govEngine,
	}
}

// SchemaRegistry returns the active schema registry.
func (h *Handlers) SchemaRegistry() *schema.Registry {
	if h.schemaReg == nil {
		h.schemaReg = schema.NewRegistry(schema.NewMemoryStore())
	}
	return h.schemaReg
}

// StreamAdapter returns the active stream adapter.
func (h *Handlers) StreamAdapter() streaming.StreamAdapter {
	if h.streamAdapter == nil {
		h.streamAdapter = streaming.NewMemoryStreamAdapter(4)
	}
	return h.streamAdapter
}

// OffsetStore returns the active stream offset store.
func (h *Handlers) OffsetStore() streaming.OffsetStore {
	if h.streamOffsets == nil {
		h.streamOffsets = streaming.NewMemoryOffsetStore()
	}
	return h.streamOffsets
}

// DeadLetterStore returns the active dead letter stream store.
func (h *Handlers) DeadLetterStore() streaming.DeadLetterStore {
	if h.streamDLQStore == nil {
		h.streamDLQStore = streaming.NewMemoryDeadLetterStore()
	}
	return h.streamDLQStore
}

// StreamReplayEngine returns the active stream replay engine.
func (h *Handlers) StreamReplayEngine() *streaming.ReplayEngine {
	if h.streamReplay == nil {
		producer, _ := h.StreamAdapter().Producer()
		h.streamReplay = streaming.NewReplayEngine(h.events, h.workflowEngine, h.DeadLetterStore(), producer)
	}
	return h.streamReplay
}

// ConnectorRegistry returns the active connector registry.
func (h *Handlers) ConnectorRegistry() *connector.Registry {
	if h.connectorReg == nil {
		h.connectorReg = connector.Default()
	}
	return h.connectorReg
}

// OAuthManager returns the active OAuth manager.
func (h *Handlers) OAuthManager() *oauth.Manager {
	if h.oauthManager == nil {
		h.oauthManager = oauth.NewManager(h.ConnectorRegistry(), h.creds, nil)
	}
	return h.oauthManager
}

// LiveHub returns the operational event broadcast hub.
func (h *Handlers) LiveHub() *LiveHub {
	if h.liveHub == nil {
		h.liveHub = NewLiveHub()
	}
	return h.liveHub
}

// SetErrorTracker sets the error tracker provider.
func (h *Handlers) SetErrorTracker(tracker errtracker.Provider) {
	h.errorTracker = tracker
}

func (h *Handlers) Tracker() errtracker.Provider {
	if h.errorTracker != nil {
		return h.errorTracker
	}
	return errtracker.NewLogProvider()
}

// NewHandlers provides backwards-compatible initialization.
func NewHandlers(
	srcStore sources.Store,
	destStore destinations.Store,
	routeStore routing.Store,
	eventStore event.Store,
	deliveryStore delivery.Store,
	pipeline *workflow.Pipeline,
	sourceReg *sources.Registry,
	destReg *destinations.Registry,
) *Handlers {
	return &Handlers{
		sources:      srcStore,
		destinations: destStore,
		routes:       routeStore,
		events:       eventStore,
		deliveries:   deliveryStore,
		pipeline:     pipeline,
		sourceReg:    sourceReg,
		destReg:      destReg,
	}
}

// SetTelegramAPI configures the Telegram API client for live access checks.
func (h *Handlers) SetTelegramAPI(api telegram.API) {
	h.telegramAPI = api
}

// SetWorkflowServices wires the workflow engine and related services.
func (h *Handlers) SetWorkflowServices(
	wfStore workflow.Store,
	execStore workflow.ExecutionStore,
	dlStore workflow.DeadLetterStore,
	engine *workflow.Engine,
	actionReg *workflow.ActionRegistry,
	credStore credentials.Store,
	whStore webhooks.Store,
) {
	h.workflows = wfStore
	h.executions = execStore
	h.deadLetters = dlStore
	h.workflowEngine = engine
	h.actionReg = actionReg
	h.creds = credStore
	h.webhookStore = whStore
}

// Health handlers
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	circuitBreakers := map[string]string{}
	status := "healthy"
	if h.workflowEngine != nil && h.workflowEngine.CircuitBreakers() != nil {
		circuitBreakers = h.workflowEngine.CircuitBreakers().Status()
		for _, state := range circuitBreakers {
			if state == "open" {
				status = "degraded"
				break
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":           status,
		"version":          version.Version,
		"circuit_breakers": circuitBreakers,
	})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	registeredSources := []string{}
	if h.sourceReg != nil {
		registeredSources = h.sourceReg.RegisteredTypes()
	}
	registeredDestinations := []string{}
	if h.destReg != nil {
		registeredDestinations = h.destReg.RegisteredTypes()
	}
	registeredActions := []string{}
	if h.actionReg != nil {
		registeredActions = h.actionReg.RegisteredTypes()
	}

	circuitBreakers := map[string]string{}
	if h.workflowEngine != nil && h.workflowEngine.CircuitBreakers() != nil {
		circuitBreakers = h.workflowEngine.CircuitBreakers().Status()
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":                  "ready",
		"version":                 version.Version,
		"registered_sources":      registeredSources,
		"registered_destinations": registeredDestinations,
		"registered_actions":      registeredActions,
		"circuit_breakers":        circuitBreakers,
	})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondErrorWithCode(w, status, "ERROR", message, nil)
}
