package tests

import (
	"time"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/delivery"
	"kerob/internal/core/routing"
	"kerob/internal/core/workflow"
	"kerob/internal/telegram"
)

func setupTelegramEngineTest(mockAPI *telegram.MockAPI) (
	*MemorySourceStore,
	*MemoryDestinationStore,
	*MemoryRouteStore,
	*MemoryEventStore,
	*MemoryDeliveryStore,
	*workflow.Pipeline,
	*delivery.Worker,
) {
	evStore := NewMemoryEventStore()
	srcStore := NewMemorySourceStore()
	destStore := NewMemoryDestinationStore()
	routeStore := NewMemoryRouteStore()
	jobStore := NewMemoryDeliveryStore()

	srcReg := sources.NewRegistry()
	srcReg.Register("telegram", func() sources.SourceAdapter {
		return sources.NewTelegramSourceAdapter()
	})
	srcReg.Register("telegram_public", func() sources.SourceAdapter {
		return sources.NewTelegramPublicSourceAdapter()
	})

	destReg := destinations.NewRegistry()
	destReg.Register("telegram", func() destinations.DestinationAdapter {
		return destinations.NewTelegramDestinationAdapterWithAPI(mockAPI)
	})

	rEngine := routing.NewRoutingEngine(routeStore)
	pipe := workflow.NewPipeline(srcStore, srcReg, evStore, rEngine, jobStore, destStore, 3)

	worker := delivery.NewWorker(jobStore, evStore, destStore, destReg, delivery.WorkerConfig{
		PollInterval:   10 * time.Millisecond,
		Concurrency:    1,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	})

	return srcStore, destStore, routeStore, evStore, jobStore, pipe, worker
}
