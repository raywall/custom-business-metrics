package application

import (
	"context"
	"time"

	"custom-business-metrics/service/internal/core"
)

// MetricService coordinates metric ingestion and query use cases.
type MetricService struct {
	repo   MetricRepository
	config *ConfigService
	now    func() time.Time
}

// NewMetricService creates a metric use-case service.
func NewMetricService(repo MetricRepository, config *ConfigService) *MetricService {
	return &MetricService{repo: repo, config: config, now: time.Now}
}

// Ingest validates and stores metric events.
func (s *MetricService) Ingest(ctx context.Context, events []core.MetricEvent) error {
	now := s.now().UTC()
	for i := range events {
		if err := events[i].Validate(now); err != nil {
			return err
		}
	}
	return s.repo.SaveMetrics(ctx, events, s.config.Get().RetentionDays)
}

// Events returns raw metric events for traceability and historical validation.
func (s *MetricService) Events(ctx context.Context, filter core.MetricFilter, limit int) ([]core.MetricEventRecord, error) {
	normalizeFilter(&filter, s.now)
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	return s.repo.ListEvents(ctx, filter, limit)
}

// Summaries returns aggregated metric summaries.
func (s *MetricService) Summaries(ctx context.Context, filter core.MetricFilter) ([]core.MetricSummary, error) {
	normalizeFilter(&filter, s.now)
	return s.repo.ListSummaries(ctx, filter)
}

// Series returns bucketed metric data for charts.
func (s *MetricService) Series(ctx context.Context, filter core.MetricFilter, bucket time.Duration) ([]core.MetricPoint, error) {
	normalizeFilter(&filter, s.now)
	if bucket <= 0 {
		bucket = time.Minute
	}
	return s.repo.QuerySeries(ctx, filter, bucket)
}

// Dimensions returns known values for filter dimensions.
func (s *MetricService) Dimensions(ctx context.Context) (core.MetricDimensions, error) {
	return s.repo.ListDimensions(ctx)
}

func normalizeFilter(filter *core.MetricFilter, now func() time.Time) {
	if filter.To.IsZero() {
		filter.To = now().UTC()
	}
	if filter.From.IsZero() {
		filter.From = filter.To.Add(-30 * time.Minute)
	}
}
