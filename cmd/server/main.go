package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/faisalaffan/ewallet-system/docs"
	"github.com/faisalaffan/ewallet-system/internal/config"
	"github.com/faisalaffan/ewallet-system/internal/handler"
	"github.com/faisalaffan/ewallet-system/internal/repository"
	"github.com/faisalaffan/ewallet-system/internal/router"
	"github.com/faisalaffan/ewallet-system/internal/service"
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

	walletRepo := repository.NewWalletRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)

	walletSvc := service.NewWalletService(walletRepo, ledgerRepo)
	reconcileSvc := service.NewReconcileService(walletRepo, ledgerRepo)

	h := handler.NewWalletHandler(walletSvc, reconcileSvc)

	app := router.Setup(h)

	sigCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-sigCtx.Done()
		log.Println("shutting down...")
		app.Shutdown()
	}()

	log.Printf("server listening on :%s", cfg.AppPort)
	return app.Listen(":" + cfg.AppPort)
}
