package core

import (
	"errors"
	"strings"
	"time"
)

// MetricKind describes how values are interpreted by aggregations.
type MetricKind string

const (
	// MetricKindCount represents incremental counters.
	MetricKindCount MetricKind = "count"
	// MetricKindGauge represents point-in-time values.
	MetricKindGauge MetricKind = "gauge"
	// MetricKindMoney represents financial amounts.
	MetricKindMoney MetricKind = "money"
)

// MetricEvent is the unit accepted by the service and agent.
type MetricEvent struct {
	Name      string            `json:"name"`
	Kind      MetricKind        `json:"kind"`
	Value     float64           `json:"value"`
	Unit      string            `json:"unit,omitempty"`
	Segment   string            `json:"segment,omitempty"`
	Workflow  string            `json:"workflow,omitempty"`
	Step      string            `json:"step,omitempty"`
	Status    string            `json:"status,omitempty"`
	Source    string            `json:"source,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// MetricEventRecord is a stored metric event with retention metadata.
type MetricEventRecord struct {
	ID        string      `json:"id"`
	Event     MetricEvent `json:"event"`
	ExpiresAt time.Time   `json:"expiresAt"`
}

// Validate normalizes and validates a metric event before storage.
func (m *MetricEvent) Validate(now time.Time) error {
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" {
		return errors.New("metric name is required")
	}
	if m.Kind == "" {
		m.Kind = MetricKindCount
	}
	if m.Kind != MetricKindCount && m.Kind != MetricKindGauge && m.Kind != MetricKindMoney {
		return errors.New("metric kind must be count, gauge, or money")
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = now.UTC()
	} else {
		m.Timestamp = m.Timestamp.UTC()
	}
	if m.Tags == nil {
		m.Tags = map[string]string{}
	}
	normalized := map[string]string{}
	for key, value := range m.Tags {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			normalized[key] = value
		}
	}
	m.Tags = normalized
	return nil
}

// MetricFilter limits queries to a time interval and optional dimensions.
type MetricFilter struct {
	Name     string
	Segment  string
	Workflow string
	Step     string
	Status   string
	Source   string
	GroupBy  string
	Tags     map[string]string
	TagIn    map[string][]string
	From     time.Time
	To       time.Time
}

// MetricSummary contains aggregate values for a metric name.
type MetricSummary struct {
	Name      string     `json:"name"`
	Group     string     `json:"group,omitempty"`
	GroupBy   string     `json:"groupBy,omitempty"`
	Kind      MetricKind `json:"kind"`
	Unit      string     `json:"unit,omitempty"`
	Count     int        `json:"count"`
	Sum       float64    `json:"sum"`
	Average   float64    `json:"average"`
	Latest    float64    `json:"latest"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// MetricDimensions describes known dimension and tag values.
type MetricDimensions struct {
	Segments  []string            `json:"segments"`
	Workflows []string            `json:"workflows"`
	Steps     []string            `json:"steps"`
	Statuses  []string            `json:"statuses"`
	Sources   []string            `json:"sources"`
	TagKeys   []string            `json:"tagKeys"`
	Tags      map[string][]string `json:"tags"`
}

// MetricPoint is an aggregated time-series bucket.
type MetricPoint struct {
	Bucket time.Time `json:"bucket"`
	Value  float64   `json:"value"`
	Count  int       `json:"count"`
}

// RuntimeConfig stores operational settings that affect ingestion and query.
type RuntimeConfig struct {
	RetentionDays int `json:"retentionDays"`
}

// Dashboard stores a dashboard definition managed by users.
type Dashboard struct {
	ID             string            `json:"id"`
	SchemaVersion  int               `json:"schemaVersion"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	RefreshSeconds int               `json:"refreshSeconds"`
	Variables      []DashboardVar    `json:"variables,omitempty"`
	Widgets        []DashboardWidget `json:"widgets"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

// DashboardVar describes a reusable dashboard parameter.
type DashboardVar struct {
	Name    string   `json:"name"`
	Label   string   `json:"label,omitempty"`
	Type    string   `json:"type,omitempty"`
	Default string   `json:"default,omitempty"`
	Values  []string `json:"values,omitempty"`
}

// DashboardWidget describes one metric visualization.
type DashboardWidget struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Title       string            `json:"title"`
	Metric      string            `json:"metric"`
	Query       string            `json:"query"`
	Aggregation string            `json:"aggregation"`
	Chart       string            `json:"chart"`
	GroupBy     string            `json:"groupBy,omitempty"`
	Filters     map[string]string `json:"filters,omitempty"`
	TagFilters  map[string]string `json:"tagFilters,omitempty"`
	Layout      WidgetLayout      `json:"layout"`
	Display     WidgetDisplay     `json:"display"`
}

// WidgetLayout defines dashboard grid placement.
type WidgetLayout struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// WidgetDisplay defines rendering options for a widget.
type WidgetDisplay struct {
	Label       string `json:"label,omitempty"`
	Unit        string `json:"unit,omitempty"`
	Format      string `json:"format,omitempty"`
	Legend      bool   `json:"legend,omitempty"`
	ShowHeader  bool   `json:"showHeader,omitempty"`
	DecimalSize int    `json:"decimalSize,omitempty"`
}
