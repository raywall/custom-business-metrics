package service_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	metricsservice "github.com/raywall/custom-business-metrics/service"
)

func TestServiceHandlerIngestsAndQueriesMetrics(t *testing.T) {
	t.Parallel()

	service, err := metricsservice.New(context.Background(), metricsservice.Config{
		StorageBackend: metricsservice.StorageMemory,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	ingest := httptest.NewRequest(http.MethodPost, "/v1/metrics", bytes.NewBufferString(`{
		"events": [{
			"name": "orders.processed",
			"kind": "count",
			"value": 1,
			"workflow": "order-processing",
			"status": "completed",
			"tags": {"correlation_id": "corr-example-001"}
		}]
	}`))
	ingest.Header.Set("Content-Type", "application/json")
	ingestResponse := httptest.NewRecorder()
	service.Handler().ServeHTTP(ingestResponse, ingest)
	if ingestResponse.Code != http.StatusAccepted {
		t.Fatalf("ingest status: got %d, want %d; body=%s", ingestResponse.Code, http.StatusAccepted, ingestResponse.Body.String())
	}

	query := httptest.NewRequest(http.MethodGet, "/v1/metrics/events?correlation_id=corr-example-001", nil)
	queryResponse := httptest.NewRecorder()
	service.Handler().ServeHTTP(queryResponse, query)
	if queryResponse.Code != http.StatusOK {
		t.Fatalf("query status: got %d, want %d", queryResponse.Code, http.StatusOK)
	}
	if !bytes.Contains(queryResponse.Body.Bytes(), []byte("orders.processed")) {
		t.Fatalf("query response does not contain ingested metric: %s", queryResponse.Body.String())
	}
}

func TestNewRejectsUnknownStorage(t *testing.T) {
	t.Parallel()

	_, err := metricsservice.New(context.Background(), metricsservice.Config{StorageBackend: "unknown"})
	if err == nil {
		t.Fatal("expected unsupported storage error")
	}
}
