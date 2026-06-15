package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	metrics "github.com/raywall/custom-business-metrics/agent"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	agent, err := metrics.New(metrics.Config{
		ServiceEndpoint: env("METRICS_SERVICE_ENDPOINT", "http://localhost:8080/v1/metrics"),
		BatchSize:       10,
		FlushInterval:   time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- agent.Run(ctx) }()

	for _, step := range []string{"validate", "reserve_inventory", "notify_customer"} {
		if err := agent.Emit(ctx, metricEvent(step)); err != nil {
			log.Printf("emit metric: %v", err)
		}
	}

	agent.Close()
	if err := <-done; err != nil && err != context.Canceled {
		log.Fatal(err)
	}
	cancel()
	fmt.Println("metricas enviadas")
}

func metricEvent(step string) map[string]any {
	return map[string]any{
		"name":      "routing_slip.step.completed",
		"kind":      "count",
		"value":     1,
		"unit":      "event",
		"workflow":  "order-processing",
		"step":      step,
		"status":    "completed",
		"source":    "importable-agent-example",
		"trace_id":  "trace-example-001",
		"timestamp": time.Now().UTC(),
		"tags": map[string]string{
			"correlation_id": "corr-example-001",
			"order_id":       "ORD-1001",
		},
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
