package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/raywall/custom-business-metrics/service/internal/adapters/dynamodbstore"
	"github.com/raywall/custom-business-metrics/service/internal/adapters/httpapi"
	"github.com/raywall/custom-business-metrics/service/internal/adapters/memory"
	"github.com/raywall/custom-business-metrics/service/internal/application"
)

const (
	// StorageMemory keeps metrics in process memory and is intended for development and tests.
	StorageMemory = "memory"
	// StorageDynamoDB persists metrics in an existing DynamoDB table.
	StorageDynamoDB = "dynamodb"
)

// Config controls the embeddable metrics HTTP service.
type Config struct {
	StorageBackend string
	RetentionDays  int
	DynamoDBTable  string
	AWSRegion      string
	DynamoEndpoint string
	Logger         *slog.Logger
}

// Service provides the Custom Business Metrics HTTP API as an embeddable handler.
type Service struct {
	handler http.Handler
}

// New creates an embeddable metrics service using memory or DynamoDB storage.
func New(ctx context.Context, config Config) (*Service, error) {
	if config.RetentionDays <= 0 {
		config.RetentionDays = 7
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	store, err := buildStore(ctx, config)
	if err != nil {
		return nil, err
	}
	runtimeConfig := application.NewConfigService(config.RetentionDays)
	metrics := application.NewMetricService(store, runtimeConfig)
	dashboards := application.NewDashboardService(store)
	handler := httpapi.NewServer(metrics, dashboards, runtimeConfig, config.Logger).Handler()
	return &Service{handler: handler}, nil
}

// Handler returns the HTTP API handler for mounting in an existing server, Lambda adapter, or test.
func (s *Service) Handler() http.Handler {
	return s.handler
}

func buildStore(ctx context.Context, config Config) (interface {
	application.MetricRepository
	application.DashboardRepository
}, error) {
	switch strings.ToLower(strings.TrimSpace(config.StorageBackend)) {
	case "", StorageMemory:
		return memory.NewStore(), nil
	case StorageDynamoDB:
		if strings.TrimSpace(config.DynamoDBTable) == "" {
			return nil, fmt.Errorf("dynamodb table is required")
		}
		return dynamodbstore.NewStore(ctx, config.DynamoDBTable, config.AWSRegion, config.DynamoEndpoint)
	default:
		return nil, fmt.Errorf("unsupported storage backend %q", config.StorageBackend)
	}
}
