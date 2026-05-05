package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"custom-business-metrics/service/internal/adapters/dynamodbstore"
	"custom-business-metrics/service/internal/adapters/httpapi"
	"custom-business-metrics/service/internal/adapters/memory"
	"custom-business-metrics/service/internal/application"
)

// main starts the Custom Business Metrics service.
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	addr := env("SERVICE_ADDR", ":8080")
	configService := application.NewConfigService(envInt("RETENTION_DAYS", 7))

	store, err := buildStore(context.Background(), logger)
	if err != nil {
		logger.Error("storage initialization failed", "error", err)
		os.Exit(1)
	}
	metricService := application.NewMetricService(store, configService)
	dashboardService := application.NewDashboardService(store)
	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewServer(metricService, dashboardService, configService, logger).Handler(),
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

func envInt(key string, fallback int) int {
	value := env(key, "")
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func buildStore(ctx context.Context, logger *slog.Logger) (interface {
	application.MetricRepository
	application.DashboardRepository
}, error) {
	if env("STORAGE_BACKEND", "memory") != "dynamodb" {
		logger.Info("storage selected", "backend", "memory")
		return memory.NewStore(), nil
	}
	logger.Info("storage selected", "backend", "dynamodb")
	return dynamodbstore.NewStore(
		ctx,
		env("DYNAMODB_TABLE", "custom-business-metrics-events"),
		env("AWS_REGION", "us-east-1"),
		env("DYNAMODB_ENDPOINT", ""),
	)
}
