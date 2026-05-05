package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"custom-business-metrics/service/internal/core"
)

// DashboardService coordinates dashboard management use cases.
type DashboardService struct {
	repo DashboardRepository
	now  func() time.Time
}

// NewDashboardService creates a dashboard use-case service.
func NewDashboardService(repo DashboardRepository) *DashboardService {
	return &DashboardService{repo: repo, now: time.Now}
}

// List returns all dashboards.
func (s *DashboardService) List(ctx context.Context) ([]core.Dashboard, error) {
	return s.repo.ListDashboards(ctx)
}

// Save validates and stores a dashboard definition.
func (s *DashboardService) Save(ctx context.Context, dashboard core.Dashboard) (core.Dashboard, error) {
	now := s.now().UTC()
	dashboard.Name = strings.TrimSpace(dashboard.Name)
	if dashboard.Name == "" {
		dashboard.Name = "Operational dashboard"
	}
	if dashboard.ID == "" {
		dashboard.ID = newID()
		dashboard.CreatedAt = now
	}
	if dashboard.SchemaVersion == 0 {
		dashboard.SchemaVersion = 1
	}
	if dashboard.CreatedAt.IsZero() {
		dashboard.CreatedAt = now
	}
	if dashboard.RefreshSeconds <= 0 {
		dashboard.RefreshSeconds = 5
	}
	dashboard.UpdatedAt = now
	for i := range dashboard.Widgets {
		if dashboard.Widgets[i].ID == "" {
			dashboard.Widgets[i].ID = newID()
		}
		if dashboard.Widgets[i].Type == "" {
			dashboard.Widgets[i].Type = "timeseries"
		}
		if dashboard.Widgets[i].Aggregation == "" {
			dashboard.Widgets[i].Aggregation = "sum"
		}
		if dashboard.Widgets[i].Chart == "" {
			dashboard.Widgets[i].Chart = dashboard.Widgets[i].Type
		}
		if dashboard.Widgets[i].Query == "" && dashboard.Widgets[i].Metric != "" {
			dashboard.Widgets[i].Query = dashboard.Widgets[i].Aggregation + ":" + dashboard.Widgets[i].Metric + "{}.as_count()"
		}
		if dashboard.Widgets[i].Layout.W <= 0 {
			dashboard.Widgets[i].Layout.W = 6
		}
		if dashboard.Widgets[i].Layout.H <= 0 {
			dashboard.Widgets[i].Layout.H = 3
		}
	}
	return s.repo.SaveDashboard(ctx, dashboard)
}

// Delete removes a dashboard definition.
func (s *DashboardService) Delete(ctx context.Context, id string) error {
	return s.repo.DeleteDashboard(ctx, id)
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(b[:])
}
