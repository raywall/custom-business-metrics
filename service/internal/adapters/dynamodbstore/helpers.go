package dynamodbstore

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"time"

	"custom-business-metrics/service/internal/core"
)

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

func eventID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
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
