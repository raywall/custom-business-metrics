package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"custom-business-metrics/service/internal/core"
)

// Store is an in-memory implementation of all repository ports.
type Store struct {
	mu         sync.RWMutex
	events     []core.MetricEvent
	dashboards map[string]core.Dashboard
}

// NewStore creates an empty in-memory repository.
func NewStore() *Store {
	return &Store{dashboards: seedDashboards()}
}

// SaveMetrics appends metric events.
func (s *Store) SaveMetrics(_ context.Context, events []core.MetricEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
	return nil
}

// ListSummaries returns aggregates grouped by metric name.
func (s *Store) ListSummaries(_ context.Context, filter core.MetricFilter) ([]core.MetricSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byKey := map[string]*core.MetricSummary{}
	for _, event := range s.events {
		if !matches(event, filter) {
			continue
		}
		group := groupValue(event, filter.GroupBy)
		key := event.Name + "\x00" + filter.GroupBy + "\x00" + group
		summary := byKey[key]
		if summary == nil {
			summary = &core.MetricSummary{Name: event.Name, Group: group, GroupBy: filter.GroupBy, Kind: event.Kind, Unit: event.Unit}
			byKey[key] = summary
		}
		summary.Count++
		summary.Sum += event.Value
		summary.Latest = event.Value
		if event.Timestamp.After(summary.UpdatedAt) {
			summary.UpdatedAt = event.Timestamp
			summary.Latest = event.Value
		}
	}

	out := make([]core.MetricSummary, 0, len(byKey))
	for _, summary := range byKey {
		if summary.Count > 0 {
			summary.Average = summary.Sum / float64(summary.Count)
		}
		out = append(out, *summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Group < out[j].Group
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// QuerySeries returns sum/count points grouped by bucket.
func (s *Store) QuerySeries(_ context.Context, filter core.MetricFilter, bucket time.Duration) ([]core.MetricPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byBucket := map[int64]*core.MetricPoint{}
	for _, event := range s.events {
		if !matches(event, filter) {
			continue
		}
		key := event.Timestamp.Truncate(bucket).Unix()
		point := byBucket[key]
		if point == nil {
			point = &core.MetricPoint{Bucket: time.Unix(key, 0).UTC()}
			byBucket[key] = point
		}
		point.Value += event.Value
		point.Count++
	}

	out := make([]core.MetricPoint, 0, len(byBucket))
	for _, point := range byBucket {
		out = append(out, *point)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bucket.Before(out[j].Bucket) })
	return out, nil
}

// ListDimensions returns known dimension and tag values for filters.
func (s *Store) ListDimensions(_ context.Context) (core.MetricDimensions, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sets := map[string]map[string]struct{}{
		"segments":  {},
		"workflows": {},
		"steps":     {},
		"statuses":  {},
		"sources":   {},
	}
	tagSets := map[string]map[string]struct{}{}
	for _, event := range s.events {
		add(sets["segments"], event.Segment)
		add(sets["workflows"], event.Workflow)
		add(sets["steps"], event.Step)
		add(sets["statuses"], event.Status)
		add(sets["sources"], event.Source)
		for key, value := range event.Tags {
			if tagSets[key] == nil {
				tagSets[key] = map[string]struct{}{}
			}
			add(tagSets[key], value)
		}
	}

	tags := map[string][]string{}
	tagKeys := make([]string, 0, len(tagSets))
	for key, set := range tagSets {
		tagKeys = append(tagKeys, key)
		tags[key] = sortedValues(set)
	}
	sort.Strings(tagKeys)

	return core.MetricDimensions{
		Segments:  sortedValues(sets["segments"]),
		Workflows: sortedValues(sets["workflows"]),
		Steps:     sortedValues(sets["steps"]),
		Statuses:  sortedValues(sets["statuses"]),
		Sources:   sortedValues(sets["sources"]),
		TagKeys:   tagKeys,
		Tags:      tags,
	}, nil
}

// ListDashboards returns all dashboard definitions.
func (s *Store) ListDashboards(_ context.Context) ([]core.Dashboard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.Dashboard, 0, len(s.dashboards))
	for _, dashboard := range s.dashboards {
		out = append(out, dashboard)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SaveDashboard creates or replaces a dashboard definition.
func (s *Store) SaveDashboard(_ context.Context, dashboard core.Dashboard) (core.Dashboard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dashboards[dashboard.ID] = dashboard
	return dashboard, nil
}

// DeleteDashboard removes a dashboard definition.
func (s *Store) DeleteDashboard(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.dashboards, id)
	return nil
}

func matches(event core.MetricEvent, filter core.MetricFilter) bool {
	if !filter.From.IsZero() && event.Timestamp.Before(filter.From) {
		return false
	}
	if !filter.To.IsZero() && event.Timestamp.After(filter.To) {
		return false
	}
	return match(filter.Name, event.Name) &&
		match(filter.Segment, event.Segment) &&
		match(filter.Workflow, event.Workflow) &&
		match(filter.Step, event.Step) &&
		match(filter.Status, event.Status) &&
		match(filter.Source, event.Source) &&
		matchTags(filter.Tags, event.Tags) &&
		matchTagIn(filter.TagIn, event.Tags)
}

func match(want, got string) bool {
	return want == "" || want == got
}

func matchTags(want, got map[string]string) bool {
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

func matchTagIn(want map[string][]string, got map[string]string) bool {
	for key, values := range want {
		if len(values) == 0 {
			continue
		}
		found := false
		for _, value := range values {
			if got[key] == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func groupValue(event core.MetricEvent, groupBy string) string {
	switch groupBy {
	case "":
		return ""
	case "segment":
		return event.Segment
	case "workflow":
		return event.Workflow
	case "step":
		return event.Step
	case "status":
		return event.Status
	case "source":
		return event.Source
	default:
		const prefix = "tag:"
		if len(groupBy) > len(prefix) && groupBy[:len(prefix)] == prefix {
			return event.Tags[groupBy[len(prefix):]]
		}
		return ""
	}
}

func add(set map[string]struct{}, value string) {
	if value != "" {
		set[value] = struct{}{}
	}
}

func sortedValues(set map[string]struct{}) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func seedDashboards() map[string]core.Dashboard {
	now := time.Now().UTC()
	dashboard := core.Dashboard{
		ID:             "operations",
		SchemaVersion:  1,
		Name:           "Operacao de parcelas",
		Description:    "Visao realtime de baixa e ressarcimento por segmento.",
		RefreshSeconds: 5,
		CreatedAt:      now,
		UpdatedAt:      now,
		Widgets: []core.DashboardWidget{
			{ID: "processed", Type: "indicator", Title: "Parcelas processadas", Metric: "installments.processed", Query: "sum:installments.processed{}.as_count()", Aggregation: "sum", Chart: "indicator", Layout: core.WidgetLayout{X: 0, Y: 0, W: 3, H: 2}},
			{ID: "by-step", Type: "bar", Title: "Processamento por etapa", Metric: "installments.processed", Query: "sum:installments.processed{} by {tag:etapa}.as_count()", Aggregation: "sum", Chart: "bar", GroupBy: "tag:etapa", Layout: core.WidgetLayout{X: 3, Y: 0, W: 5, H: 3}},
			{ID: "by-result", Type: "table", Title: "Resultados finais", Metric: "installments.result", Query: "sum:installments.processed{} by {tag:result}.as_count()", Aggregation: "sum", Chart: "table", GroupBy: "tag:result", Layout: core.WidgetLayout{X: 8, Y: 0, W: 4, H: 3}},
			{ID: "reprocess", Type: "list", Title: "Reprocessamentos", Metric: "installments.processed", Query: "sum:installments.processed{processing_count in(2,3,4,5)} by {tag:processing_count}.as_count()", Aggregation: "sum", Chart: "list", GroupBy: "tag:processing_count", Layout: core.WidgetLayout{X: 0, Y: 3, W: 6, H: 3}},
		},
	}
	return map[string]core.Dashboard{dashboard.ID: dashboard}
}
