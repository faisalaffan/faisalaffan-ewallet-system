package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/faisalaffan/ewallet-system/docs"
	"github.com/faisalaffan/ewallet-system/internal/config"
	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/faisalaffan/ewallet-system/internal/handler"
	"github.com/faisalaffan/ewallet-system/internal/repository"
	"github.com/faisalaffan/ewallet-system/internal/router"
	"github.com/faisalaffan/ewallet-system/internal/service"
	"github.com/faisalaffan/ewallet-system/internal/worker"
	"github.com/faisalaffan/ewallet-system/pkg/database"
	"github.com/faisalaffan/ewallet-system/pkg/telemetry"
)

func main() {
	if err := run(); err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	otelServiceName := os.Getenv("OTEL_SERVICE_NAME")
	if otelServiceName == "" {
		otelServiceName = "ewallet-api"
	}

	telemetry.LoggerSetup(otelServiceName)

	docs.SwaggerInfo.BasePath = "/api"
	docs.SwaggerInfo.Host = os.Getenv("SWAGGER_HOST")
	docs.SwaggerInfo.Schemes = []string{"https", "http"}

	shutdown, err := telemetry.TracerSetup(ctx, otelServiceName)
	if err != nil {
		slog.Warn("failed to setup OTel tracing", "error", err)
	} else {
		defer func() {
			if err := shutdown(ctx); err != nil {
				slog.Warn("failed to shutdown OTel", "error", err)
			}
		}()
	}

	if err := telemetry.MeterSetup(); err != nil {
		slog.Warn("failed to setup OTel metrics", "error", err)
	}

	db, err := database.NewPostgres(cfg.DSN())
	if err != nil {
		return err
	}

	if err := database.AddOtelPlugin(db); err != nil {
		slog.Warn("failed to add OTel plugin to GORM", "error", err)
	}

	if err := database.RunMigrations(db); err != nil {
		return err
	}

	// EventBus: channel-based pub/sub for ledger events
	eventBus := worker.NewEventBus()
	slog.Info("event-bus initialized")

	// Subscriber goroutine: log ledger events asynchronously
	eventBus.Add(1)
	ledgerCh := eventBus.Subscribe("ledger", 256)
	go func() {
		defer eventBus.Done()
		for raw := range ledgerCh {
			evt, ok := raw.(domain.LedgerEntry)
			if !ok {
				continue
			}
			slog.Info("ledger event",
				"wallet_id", evt.WalletID.String(),
				"entry_type", evt.EntryType,
				"amount", evt.Amount,
			)
		}
		slog.Info("event-bus subscriber stopped")
	}()

	walletRepo := repository.NewWalletRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)

	walletSvc := service.NewWalletService(walletRepo, ledgerRepo, eventBus)
	reconcileSvc := service.NewReconcileService(walletRepo, ledgerRepo)

	h := handler.NewWalletHandler(walletSvc, reconcileSvc)

	app := router.Setup(h)

	// Background reconcile worker (goroutine + ticker)
	reconcileWorker := worker.NewReconcileWorker(db, 1*time.Hour)
	sigCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	reconcileWorker.Start(sigCtx)

	// WaitGroup-based graceful shutdown
	var shutdownWg sync.WaitGroup
	shutdownWg.Add(1)
	go func() {
		defer shutdownWg.Done()
		<-sigCtx.Done()
		slog.Info("shutdown signal received, draining...")

		if err := app.Shutdown(); err != nil {
			slog.Warn("fiber shutdown error", "error", err)
		}

		reconcileWorker.Shutdown()
		eventBus.Close()

		slog.Info("shutdown complete")
	}()

	slog.Info("server listening", "port", cfg.AppPort)
	return app.Listen(":" + cfg.AppPort)
}
