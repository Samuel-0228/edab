package api

import (
	"encoding/json"
	"net/http"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/credentials"
	"kerob/internal/core/delivery"
	"kerob/internal/core/errtracker"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/core/webhooks"
	"kerob/internal/core/workflow"
	"kerob/internal/telegram"
	"kerob/internal/version"
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
	return &Handlers{
		workflows:      cfg.Workflows,
		executions:     cfg.Executions,
		deadLetters:    cfg.DeadLetters,
		workflowEngine: cfg.WorkflowEngine,
		actionReg:      cfg.ActionReg,
		creds:          cfg.Creds,
		webhookStore:   cfg.WebhookStore,

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
	}
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
	respondJSON(w, http.StatusOK, map[string]string{
		"status":  "healthy",
		"version": version.Version,
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

	respondJSON(w, http.StatusOK, map[string]any{
		"status":                  "ready",
		"version":                 version.Version,
		"registered_sources":      registeredSources,
		"registered_destinations": registeredDestinations,
		"registered_actions":      registeredActions,
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
