package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"custom-business-metrics/service/internal/adapters/httpapi"
	"custom-business-metrics/service/internal/adapters/memory"
	"custom-business-metrics/service/internal/application"
)

// main starts the Custom Business Metrics service.
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	addr := env("SERVICE_ADDR", ":8080")

	store := memory.NewStore()
	metricService := application.NewMetricService(store)
	dashboardService := application.NewDashboardService(store)
	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewServer(metricService, dashboardService, logger).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		logger.Info("service started", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("service failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	logger.Info("service stopped")
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
