package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kerob/internal/adapters/actions"
	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/api"
	"kerob/internal/config"
	"kerob/internal/core/credentials"
	"kerob/internal/core/delivery"
	"kerob/internal/core/errtracker"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/core/webhooks"
	"kerob/internal/core/workflow"
	"kerob/internal/database"
	"kerob/internal/observability"
	"kerob/internal/telegram"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}
	setupLogger(cfg.LogLevel)

	slog.Info("starting kerob automation engine",
		slog.String("environment", cfg.Environment),
		slog.String("port", cfg.ServerPort),
	)

	// 1. Connect to PostgreSQL
	db, err := database.Connect(database.Config{
		URL: cfg.DatabaseURL,
	})
	if err != nil {
		slog.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	// 2. Run Database Migrations
	migrationCtx, cancelMigration := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelMigration()

	if err := database.Migrate(migrationCtx, db); err != nil {
		slog.Error("database migration failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("database migrations verified successfully")

	// 3. Initialize Stores
	encKey := credentials.DeriveKey(cfg.EncryptionKey)
	credStore := credentials.NewPostgresStore(db, encKey)
	wfStore := workflow.NewPostgresStore(db)
	execStore := workflow.NewPostgresExecutionStore(db)
	dlStore := workflow.NewPostgresDeadLetterStore(db)
	whStore := webhooks.NewPostgresStore(db)

	srcStore := sources.NewPostgresStore(db)
	destStore := destinations.NewPostgresStore(db)
	routeStore := routing.NewPostgresStore(db)
	eventStore := event.NewPostgresStore(db)
	deliveryStore := delivery.NewPostgresStore(db)

	// 4. Register Source and Destination Adapters
	srcRegistry := sources.NewRegistry()
	srcRegistry.Register("mock", func() sources.SourceAdapter { return sources.NewMockSourceAdapter() })
	srcRegistry.Register("webhook", func() sources.SourceAdapter { return sources.NewWebhookSourceAdapter() })
	srcRegistry.Register("telegram", func() sources.SourceAdapter { return sources.NewTelegramSourceAdapter() })
	srcRegistry.Register("telegram_public", func() sources.SourceAdapter { return sources.NewTelegramPublicSourceAdapter() })

	destRegistry := destinations.NewRegistry()
	destRegistry.Register("mock", func() destinations.DestinationAdapter { return destinations.NewMockDestinationAdapter() })
	destRegistry.Register("webhook", func() destinations.DestinationAdapter { return destinations.NewWebhookDestinationAdapter() })
	destRegistry.Register("stdout", func() destinations.DestinationAdapter { return destinations.NewStdoutDestinationAdapter() })
	destRegistry.Register("telegram", func() destinations.DestinationAdapter { return destinations.NewTelegramDestinationAdapter() })

	// 5. Initialize Workflow Action Registry & Engine
	actionRegistry := workflow.NewActionRegistry()
	actionRegistry.Register(actions.NewHTTPActionExecutor())
	actionRegistry.Register(actions.NewDiscordActionExecutor())
	actionRegistry.Register(actions.NewGitHubActionExecutor())

	var teleAPI telegram.API
	if cfg.TelegramEnabled && cfg.TelegramBotToken != "" {
		teleClient, err := telegram.NewClient(cfg.TelegramBotToken, telegram.ClientOptions{
			APIBaseURL:  cfg.TelegramAPIBaseURL,
			GlobalRate:  cfg.TelegramRateGlobal,
			PerChatRate: cfg.TelegramRatePerChat,
		})
		if err != nil {
			slog.Error("failed to initialize telegram client", slog.String("error", err.Error()))
			os.Exit(1)
		}
		teleAPI = teleClient
	}

	actionRegistry.Register(actions.NewTelegramActionExecutor(teleAPI))

	wfEngine := workflow.NewEngine(
		wfStore,
		execStore,
		dlStore,
		credStore,
		actionRegistry,
		workflow.EngineConfig{
			BaseBackoff: cfg.InitialBackoff,
			MaxBackoff:  cfg.MaxBackoff,
		},
	)
	wfEngine.SetErrorTracker(errtracker.NewLogProvider())

	// 6. Initialize Engine Pipeline & Delivery Worker
	routingEngine := routing.NewRoutingEngine(routeStore)
	pipeline := workflow.NewPipeline(
		srcStore,
		srcRegistry,
		eventStore,
		routingEngine,
		deliveryStore,
		destStore,
		cfg.MaxRetries,
	)

	worker := delivery.NewWorker(
		deliveryStore,
		eventStore,
		destStore,
		destRegistry,
		delivery.WorkerConfig{
			PollInterval:   cfg.WorkerPollInterval,
			Concurrency:    cfg.WorkerConcurrency,
			InitialBackoff: cfg.InitialBackoff,
			MaxBackoff:     cfg.MaxBackoff,
		},
	)

	// 7. Start Worker, Scheduler, and Telegram Listeners in Background
	rootCtx, stopRoot := context.WithCancel(context.Background())
	defer stopRoot()

	worker.Start(rootCtx)

	scheduler := workflow.NewScheduler(wfStore, wfEngine, 10*time.Second)
	scheduler.Start(rootCtx)

	var teleListener *telegram.Listener
	var publicPoller *telegram.PublicPoller

	teleStateStore := telegram.NewPostgresStateStore(db)

	ingestCallback := func(ctx context.Context, sourceID string, rawData []byte) error {
		ev, err := pipeline.IngestRaw(ctx, sourceID, rawData)
		if err != nil {
			return err
		}
		if ev != nil && wfEngine != nil {
			if _, err := wfEngine.ProcessEvent(ctx, ev); err != nil {
				slog.ErrorContext(ctx, "failed to process ingested event",
					slog.String("event_id", ev.ID),
					slog.String("error", err.Error()),
				)
				return err
			}
		}
		return nil
	}

	// Public channel poller: scrapes t.me/s/<channel> without needing bot in source channel
	publicResolver := sources.NewPublicSourceResolver(srcStore)
	publicPoller = telegram.NewPublicPoller(
		telegram.NewPublicScraper(),
		teleStateStore,
		telegram.IngesterFunc(ingestCallback),
		publicResolver,
		cfg.TelegramPublicPollInterval,
	)
	publicPoller.Start(rootCtx)
	slog.Info("telegram public channel poller started", slog.Duration("interval", cfg.TelegramPublicPollInterval))

	if teleAPI != nil {
		sourceResolver := sources.NewSourceResolver(srcStore)
		teleListener = telegram.NewListener(
			teleAPI,
			telegram.IngesterFunc(ingestCallback),
			sourceResolver,
			teleStateStore,
			telegram.ListenerOptions{
				PollTimeout:    cfg.TelegramPollTimeout,
				MediaGroupWait: cfg.TelegramMediaGroupWait,
			},
		)
		teleListener.Start(rootCtx)
		slog.Info("telegram bot listener started")
	} else {
		slog.Info("telegram bot listener is inactive (TELEGRAM_BOT_TOKEN not provided or TELEGRAM_ENABLED=false)")
	}

	// 8. Start REST API Server
	handlers := api.NewHandlersFromConfig(api.Config{
		Workflows:      wfStore,
		Executions:     execStore,
		DeadLetters:    dlStore,
		WorkflowEngine: wfEngine,
		ActionReg:      actionRegistry,
		Creds:          credStore,
		WebhookStore:   whStore,

		Sources:      srcStore,
		Destinations: destStore,
		Routes:       routeStore,
		Events:       eventStore,
		Deliveries:   deliveryStore,
		Pipeline:     pipeline,
		SourceReg:    srcRegistry,
		DestReg:      destRegistry,
		TelegramAPI:  teleAPI,
		ErrorTracker: errtracker.NewLogProvider(),
	})

	server := api.NewServerWithOptions(cfg.ServerPort, handlers, api.ServerOptions{
		APIToken: cfg.APIToken,
	})

	go func() {
		if err := server.Start(); err != nil {
			slog.Error("server stopped unexpectedly", slog.String("error", err.Error()))
			stopRoot()
		}
	}()

	// 9. Graceful Shutdown on SIGINT / SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		slog.Info("received termination signal", slog.String("signal", sig.String()))
	case <-rootCtx.Done():
		slog.Info("root context cancelled")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	slog.Info("shutting down kerob engine gracefully...")
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("error during server shutdown", slog.String("error", err.Error()))
	}

	stopRoot()
	scheduler.Stop()
	if publicPoller != nil {
		publicPoller.Stop()
	}
	if teleListener != nil {
		teleListener.Stop()
	}
	worker.Stop()

	slog.Info("kerob engine stopped cleanly")
}

func setupLogger(levelStr string) {
	observability.ConfigureLogger(levelStr)
}
