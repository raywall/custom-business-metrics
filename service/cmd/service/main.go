package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	metricsservice "github.com/raywall/custom-business-metrics/service"
)

// main starts the Custom Business Metrics service.
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	addr := env("SERVICE_ADDR", ":8080")
	metricsAPI, err := metricsservice.New(context.Background(), metricsservice.Config{
		StorageBackend: env("STORAGE_BACKEND", metricsservice.StorageMemory),
		RetentionDays:  envInt("RETENTION_DAYS", 7),
		DynamoDBTable:  env("DYNAMODB_TABLE", "custom-business-metrics-events"),
		AWSRegion:      env("AWS_REGION", "us-east-1"),
		DynamoEndpoint: env("DYNAMODB_ENDPOINT", ""),
		Logger:         logger,
	})
	if err != nil {
		logger.Error("storage initialization failed", "error", err)
		os.Exit(1)
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           metricsAPI.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	mcp, err := metricsservice.NewMCPServer(metricsservice.MCPConfig{
		MetricsEndpoint: "http://localhost" + addr,
		APIKey:          env("MCP_METRICS_API_KEY", ""),
		ServerAPIKey:    env("MCP_SERVER_API_KEY", ""),
	})
	if err != nil {
		logger.Error("mcp initialization failed", "error", err)
		os.Exit(1)
	}
	mcpServer := &http.Server{
		Addr:              env("MCP_ADDR", ":9093"),
		Handler:           mcp.Handler(),
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
	go func() {
		logger.Info("mcp analytics started", "addr", mcpServer.Addr)
		if err := mcpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("mcp analytics failed", "error", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	_ = mcpServer.Shutdown(shutdownCtx)
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
