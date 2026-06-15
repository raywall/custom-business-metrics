package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentEmitsBufferedMetrics(t *testing.T) {
	received := make(chan int, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Events []map[string]any `json:"events"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		received <- len(body.Events)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	agent, err := New(Config{ServiceEndpoint: server.URL, BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = agent.Run(ctx) }()
	if err := agent.Emit(ctx, map[string]any{"name": "workflow.completed", "value": 1}); err != nil {
		t.Fatal(err)
	}
	select {
	case count := <-received:
		if count != 1 {
			t.Fatalf("count = %d", count)
		}
	case <-time.After(time.Second):
		t.Fatal("metric was not delivered")
	}
}

func TestAgentRetriesFailedBatch(t *testing.T) {
	var attempts atomic.Int32
	delivered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		delivered <- struct{}{}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	agent, err := New(Config{ServiceEndpoint: server.URL, BatchSize: 1, FlushInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = agent.Run(ctx) }()
	_ = agent.Emit(ctx, map[string]any{"name": "retry"})
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("failed batch was not retried")
	}
}

func TestAgentCloseDrainsAcceptedMetrics(t *testing.T) {
	received := make(chan int, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Events []map[string]any `json:"events"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		received <- len(body.Events)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	agent, err := New(Config{ServiceEndpoint: server.URL, BatchSize: 100, FlushInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- agent.Run(context.Background()) }()

	for i := 0; i < 3; i++ {
		if err := agent.Emit(context.Background(), map[string]any{"name": "workflow.completed", "value": i}); err != nil {
			t.Fatal(err)
		}
	}
	agent.Close()

	select {
	case count := <-received:
		if count != 3 {
			t.Fatalf("count = %d, want 3", count)
		}
	case <-time.After(time.Second):
		t.Fatal("metrics were not delivered during close")
	}
	if err := <-done; err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}
