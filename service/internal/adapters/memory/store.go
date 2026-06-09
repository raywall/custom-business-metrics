package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/raywall/custom-business-metrics/service/internal/core"
)

// Store is an in-memory implementation of all repository ports.
type Store struct {
	mu         sync.RWMutex
	events     []core.MetricEventRecord
	dashboards map[string]core.Dashboard
}

// NewStore creates an empty in-memory repository.
func NewStore() *Store {
	return &Store{dashboards: seedDashboards()}
}

// SaveMetrics appends metric events using retentionDays to compute expiry.
func (s *Store) SaveMetrics(_ context.Context, events []core.MetricEvent, retentionDays int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(retentionDays) * 24 * time.Hour)
	s.pruneExpiredLocked(now)
	for _, event := range events {
		s.events = append(s.events, core.MetricEventRecord{
			ID:        newEventID(),
			Event:     event,
			ExpiresAt: expiresAt,
		})
	}
	return nil
}

// ListEvents returns raw metric events that match the filter.
func (s *Store) ListEvents(_ context.Context, filter core.MetricFilter, limit int) ([]core.MetricEventRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.pruneExpiredLocked(now)
	out := make([]core.MetricEventRecord, 0, limit)
	for _, record := range s.events {
		if !matches(record.Event, filter) {
			continue
		}
		out = append(out, record)
		if len(out) >= limit {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Event.Timestamp.Before(out[j].Event.Timestamp) })
	return out, nil
}

// ListSummaries returns aggregates grouped by metric name.
func (s *Store) ListSummaries(_ context.Context, filter core.MetricFilter) ([]core.MetricSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(time.Now().UTC())

	byKey := map[string]*core.MetricSummary{}
	for _, record := range s.events {
		event := record.Event
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(time.Now().UTC())

	byBucket := map[int64]*core.MetricPoint{}
	for _, record := range s.events {
		event := record.Event
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(time.Now().UTC())

	sets := map[string]map[string]struct{}{
		"segments":  {},
		"workflows": {},
		"steps":     {},
		"statuses":  {},
		"sources":   {},
	}
	tagSets := map[string]map[string]struct{}{}
	for _, record := range s.events {
		event := record.Event
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

func (s *Store) pruneExpiredLocked(now time.Time) {
	kept := s.events[:0]
	for _, record := range s.events {
		if record.ExpiresAt.IsZero() || record.ExpiresAt.After(now) {
			kept = append(kept, record)
		}
	}
	s.events = kept
}

func newEventID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
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
		Name:           "Processamentos",
		Description:    "Visao realtime do processamento de pedidos.",
		RefreshSeconds: 5,
		CreatedAt:      now,
		UpdatedAt:      now,
		Widgets:        []core.DashboardWidget{},
	}
	return map[string]core.Dashboard{dashboard.ID: dashboard}
}
