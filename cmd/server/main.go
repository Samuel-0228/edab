package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"automation-engine/internal/adapters/destinations"
	"automation-engine/internal/adapters/sources"
	"automation-engine/internal/api"
	"automation-engine/internal/config"
	"automation-engine/internal/core/delivery"
	"automation-engine/internal/core/event"
	"automation-engine/internal/core/routing"
	"automation-engine/internal/core/workflow"
	"automation-engine/internal/database"
	"automation-engine/internal/telegram"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}
	setupLogger(cfg.LogLevel)

	slog.Info("starting automation engine",
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
	defer db.Close()

	// 2. Run Database Migrations
	migrationCtx, cancelMigration := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelMigration()

	if err := database.Migrate(migrationCtx, db); err != nil {
		slog.Error("database migration failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("database migrations verified successfully")

	// 3. Initialize Stores
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

	// 5. Initialize Engine Components
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

	// 6. Start Worker and Telegram Listener in Background
	rootCtx, stopRoot := context.WithCancel(context.Background())
	defer stopRoot()

	worker.Start(rootCtx)

	var teleAPI telegram.API
	var teleListener *telegram.Listener
	var publicPoller *telegram.PublicPoller

	teleStateStore := telegram.NewPostgresStateStore(db)

	// Public channel poller: scrapes t.me/s/<channel> without needing bot in source channel
	publicResolver := sources.NewPublicSourceResolver(srcStore)
	publicPoller = telegram.NewPublicPoller(
		telegram.NewPublicScraper(),
		teleStateStore,
		telegram.IngesterFunc(func(ctx context.Context, sourceID string, rawData []byte) error {
			_, err := pipeline.IngestRaw(ctx, sourceID, rawData)
			return err
		}),
		publicResolver,
		cfg.TelegramPublicPollInterval,
	)
	publicPoller.Start(rootCtx)
	slog.Info("telegram public channel poller started", slog.Duration("interval", cfg.TelegramPublicPollInterval))

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

		sourceResolver := sources.NewSourceResolver(srcStore)
		teleListener = telegram.NewListener(
			teleClient,
			telegram.IngesterFunc(func(ctx context.Context, sourceID string, rawData []byte) error {
				_, err := pipeline.IngestRaw(ctx, sourceID, rawData)
				return err
			}),
			sourceResolver,
			teleStateStore,
			telegram.ListenerOptions{
				PollTimeout:    cfg.TelegramPollTimeout,
				MediaGroupWait: cfg.TelegramMediaGroupWait,
			},
		)
		teleListener.Start(rootCtx)
		slog.Info("telegram listener started")
	} else {
		slog.Info("telegram listener is inactive (TELEGRAM_BOT_TOKEN not provided or TELEGRAM_ENABLED=false)")
	}

	// 7. Start REST API Server
	handlers := api.NewHandlers(
		srcStore,
		destStore,
		routeStore,
		eventStore,
		deliveryStore,
		pipeline,
		srcRegistry,
		destRegistry,
	)
	if teleAPI != nil {
		handlers.SetTelegramAPI(teleAPI)
	}
	server := api.NewServerWithOptions(cfg.ServerPort, handlers, api.ServerOptions{
		APIToken: cfg.APIToken,
	})

	go func() {
		if err := server.Start(); err != nil {
			slog.Error("server stopped unexpectedly", slog.String("error", err.Error()))
			stopRoot()
		}
	}()

	// 8. Graceful Shutdown on SIGINT / SIGTERM
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

	slog.Info("shutting down engine gracefully...")
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("error during server shutdown", slog.String("error", err.Error()))
	}

	stopRoot()
	if publicPoller != nil {
		publicPoller.Stop()
	}
	if teleListener != nil {
		teleListener.Stop()
	}
	worker.Stop()

	slog.Info("automation engine stopped cleanly")
}

func setupLogger(levelStr string) {
	var level slog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}
