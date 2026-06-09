package core

import (
	"testing"
	"time"
)

func TestMetricEventValidateMirrorsTraceFieldsToTags(t *testing.T) {
	event := MetricEvent{
		Name:    "workflow.step.completed",
		TraceID: "trace-1",
		SpanID:  "span-1",
		Tags:    map[string]string{"correlation_id": "corr-1"},
	}

	if err := event.Validate(time.Now()); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if event.Tags["trace_id"] != "trace-1" {
		t.Fatalf("trace_id tag = %q", event.Tags["trace_id"])
	}
	if event.Tags["span_id"] != "span-1" {
		t.Fatalf("span_id tag = %q", event.Tags["span_id"])
	}
}

func TestMetricEventValidatePromotesTraceTags(t *testing.T) {
	event := MetricEvent{
		Name: "workflow.step.completed",
		Tags: map[string]string{"trace_id": "trace-2", "span_id": "span-2"},
	}

	if err := event.Validate(time.Now()); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if event.TraceID != "trace-2" {
		t.Fatalf("TraceID = %q", event.TraceID)
	}
	if event.SpanID != "span-2" {
		t.Fatalf("SpanID = %q", event.SpanID)
	}
}
