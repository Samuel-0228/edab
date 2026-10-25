package api

import (
	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/credentials"
	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/core/webhooks"
	"kerob/internal/core/workflow"
)

func setupTestServer() (*Server, *Handlers, Config) {
	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-32-byte-secret-key-1234567"))
	whStore := webhooks.NewMemoryStore()
	srcStore := sources.NewMemoryStore()
	destStore := destinations.NewMemoryStore()
	routeStore := routing.NewMemoryStore()
	eventStore := event.NewMemoryStore()
	deliveryStore := delivery.NewMemoryStore()

	srcReg := sources.NewRegistry()
	srcReg.Register("telegram", func() sources.SourceAdapter { return sources.NewMockSourceAdapter() })
	srcReg.Register("github", func() sources.SourceAdapter { return sources.NewMockSourceAdapter() })

	destReg := destinations.NewRegistry()
	destReg.Register("discord", func() destinations.DestinationAdapter { return destinations.NewMockDestinationAdapter() })
	destReg.Register("telegram", func() destinations.DestinationAdapter { return destinations.NewMockDestinationAdapter() })

	actionReg := workflow.NewActionRegistry()
	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, actionReg)

	cfg := Config{
		Workflows:      wfStore,
		Executions:     execStore,
		DeadLetters:    dlStore,
		WorkflowEngine: engine,
		ActionReg:      actionReg,
		Creds:          credStore,
		WebhookStore:   whStore,
		Sources:        srcStore,
		Destinations:   destStore,
		Routes:         routeStore,
		Events:         eventStore,
		Deliveries:     deliveryStore,
		SourceReg:      srcReg,
		DestReg:        destReg,
	}

	handlers := NewHandlersFromConfig(cfg)
	server := NewServer("0", handlers)
	return server, handlers, cfg
}
