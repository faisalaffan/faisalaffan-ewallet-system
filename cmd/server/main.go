package main

import (
	"context"
	"log"
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
		log.Fatalf("failed to start server: %v", err)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	docs.SwaggerInfo.BasePath = "/api"
	docs.SwaggerInfo.Host = os.Getenv("SWAGGER_HOST")
	docs.SwaggerInfo.Schemes = []string{"https", "http"}

	otelServiceName := os.Getenv("OTEL_SERVICE_NAME")
	if otelServiceName == "" {
		otelServiceName = "ewallet-api"
	}

	shutdown, err := telemetry.TracerSetup(ctx, otelServiceName)
	if err != nil {
		log.Printf("WARN: failed to setup OTel tracing: %v", err)
	} else {
		defer func() {
			if err := shutdown(ctx); err != nil {
				log.Printf("WARN: failed to shutdown OTel: %v", err)
			}
		}()
	}

	if err := telemetry.MeterSetup(); err != nil {
		log.Printf("WARN: failed to setup OTel metrics: %v", err)
	}

	db, err := database.NewPostgres(cfg.DSN())
	if err != nil {
		return err
	}

	if err := database.AddOtelPlugin(db); err != nil {
		log.Printf("WARN: failed to add OTel plugin to GORM: %v", err)
	}

	if err := database.RunMigrations(db); err != nil {
		return err
	}

	// EventBus: channel-based pub/sub for ledger events
	eventBus := worker.NewEventBus()
	log.Println("[event-bus] initialized")

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
			log.Printf("[event-bus] ledger: wallet=%s type=%s amount=%s",
				evt.WalletID, evt.EntryType, evt.Amount)
		}
		log.Println("[event-bus] subscriber stopped")
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
		log.Println("[shutdown] signal received, draining...")

		// Stop accepting new work
		if err := app.Shutdown(); err != nil {
			log.Printf("[shutdown] fiber shutdown error: %v", err)
		}

		// Wait for background workers
		reconcileWorker.Shutdown()
		eventBus.Close()

		log.Println("[shutdown] complete")
	}()

	log.Printf("[server] listening on :%s", cfg.AppPort)
	return app.Listen(":" + cfg.AppPort)
}
