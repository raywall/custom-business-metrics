package application

import (
	"context"
	"time"

	"custom-business-metrics/service/internal/core"
)

// MetricRepository stores metric events and serves aggregated views.
type MetricRepository interface {
	SaveMetrics(context.Context, []core.MetricEvent, int) error
	ListEvents(context.Context, core.MetricFilter, int) ([]core.MetricEventRecord, error)
	ListSummaries(context.Context, core.MetricFilter) ([]core.MetricSummary, error)
	QuerySeries(context.Context, core.MetricFilter, time.Duration) ([]core.MetricPoint, error)
	ListDimensions(context.Context) (core.MetricDimensions, error)
}

// DashboardRepository stores dashboard definitions.
type DashboardRepository interface {
	ListDashboards(context.Context) ([]core.Dashboard, error)
	SaveDashboard(context.Context, core.Dashboard) (core.Dashboard, error)
	DeleteDashboard(context.Context, string) error
}
