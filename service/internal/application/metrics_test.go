package application

import (
	"context"
	"testing"
	"time"

	"github.com/raywall/custom-business-metrics/service/internal/adapters/memory"
	"github.com/raywall/custom-business-metrics/service/internal/core"
)

func TestMetricServiceIngestAndSummaries(t *testing.T) {
	store := memory.NewStore()
	service := NewMetricService(store, NewConfigService(7))
	service.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }

	err := service.Ingest(context.Background(), []core.MetricEvent{
		{Name: "installments.processed", Kind: core.MetricKindCount, Value: 2, Segment: "EP"},
		{Name: "installments.processed", Kind: core.MetricKindCount, Value: 3, Segment: "EP"},
		{Name: "installments.error", Kind: core.MetricKindCount, Value: 1, Segment: "INSS"},
	})
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	summaries, err := service.Summaries(context.Background(), core.MetricFilter{Segment: "EP"})
	if err != nil {
		t.Fatalf("summaries failed: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Name != "installments.processed" || summaries[0].Sum != 5 || summaries[0].Count != 2 {
		t.Fatalf("unexpected summary: %+v", summaries[0])
	}
}

func TestMetricServiceRejectsInvalidKind(t *testing.T) {
	service := NewMetricService(memory.NewStore(), NewConfigService(7))
	err := service.Ingest(context.Background(), []core.MetricEvent{{Name: "x", Kind: "histogram", Value: 1}})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestMetricServiceFiltersAndGroupsByTags(t *testing.T) {
	store := memory.NewStore()
	service := NewMetricService(store, NewConfigService(7))
	service.now = func() time.Time { return time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC) }

	events := []core.MetricEvent{
		{Name: "installments.processed", Kind: core.MetricKindCount, Value: 1, Tags: map[string]string{"etapa": "iniciador", "processing_count": "1", "result": "baixa-realizada", "parcela_id": "p-1"}},
		{Name: "installments.processed", Kind: core.MetricKindCount, Value: 1, Tags: map[string]string{"etapa": "desconto-complementar", "processing_count": "2", "result": "duplicidade", "parcela_id": "p-2"}},
		{Name: "installments.processed", Kind: core.MetricKindCount, Value: 1, Tags: map[string]string{"etapa": "desconto-complementar", "processing_count": "1", "result": "baixa-realizada", "parcela_id": "p-3"}},
	}
	if err := service.Ingest(context.Background(), events); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	filtered, err := service.Summaries(context.Background(), core.MetricFilter{
		Name: "installments.processed",
		Tags: map[string]string{"etapa": "desconto-complementar"},
	})
	if err != nil {
		t.Fatalf("tag filter failed: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Count != 2 {
		t.Fatalf("unexpected filtered summaries: %+v", filtered)
	}

	grouped, err := service.Summaries(context.Background(), core.MetricFilter{
		Name:    "installments.processed",
		GroupBy: "tag:result",
	})
	if err != nil {
		t.Fatalf("tag grouping failed: %v", err)
	}
	if len(grouped) != 2 {
		t.Fatalf("expected 2 result groups, got %+v", grouped)
	}
}
