package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"time"

	"custom-business-metrics/agent/internal/collector"
	"custom-business-metrics/agent/internal/forwarder"
)

// main starts the UDP agent and HTTP forwarder.
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	events := make(chan collector.MetricEvent, 1000)
	forwardEvents := make(chan forwarder.MetricEvent, 1000)

	go func() {
		for event := range events {
			forwardEvents <- forwarder.MetricEvent(event)
		}
	}()

	udp := collector.NewUDPCollector(env("AGENT_UDP_ADDR", ":8125"), logger)
	httpForwarder := forwarder.NewHTTPForwarder(env("SERVICE_INGEST_URL", "http://localhost:8080/v1/metrics"), logger)

	go httpForwarder.Run(ctx, forwardEvents, envInt("AGENT_BATCH_SIZE", 25), envDuration("AGENT_FLUSH_INTERVAL", time.Second))
	if err := udp.Run(ctx, events); err != nil {
		logger.Error("agent stopped with error", "error", err)
		os.Exit(1)
	}
	logger.Info("agent stopped")
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(env(key, ""))
	if err != nil {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := env(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
