package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	metrics "github.com/raywall/custom-business-metrics/service"
)

func main() {
	service, err := metrics.New(context.Background(), metrics.Config{
		StorageBackend: metrics.StorageMemory,
		RetentionDays:  7,
	})
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:              env("SERVICE_ADDR", ":8080"),
		Handler:           service.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("metrics service em http://localhost%s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
