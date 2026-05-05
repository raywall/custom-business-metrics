package forwarder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// MetricEvent is the event payload sent to the service.
type MetricEvent struct {
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
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

// HTTPForwarder sends metric batches to the service ingest API.
type HTTPForwarder struct {
	url    string
	client *http.Client
	logger *slog.Logger
}

// NewHTTPForwarder creates an HTTP metric forwarder.
func NewHTTPForwarder(url string, logger *slog.Logger) *HTTPForwarder {
	return &HTTPForwarder{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
		logger: logger,
	}
}

// Run batches incoming events and forwards them periodically.
func (f *HTTPForwarder) Run(ctx context.Context, in <-chan MetricEvent, batchSize int, flushEvery time.Duration) {
	if batchSize <= 0 {
		batchSize = 25
	}
	if flushEvery <= 0 {
		flushEvery = time.Second
	}

	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()

	batch := make([]MetricEvent, 0, batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := f.send(ctx, batch); err != nil {
			f.logger.Warn("metric batch failed", "count", len(batch), "error", err)
		} else {
			f.logger.Info("metric batch sent", "count", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case event := <-in:
			batch = append(batch, event)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func (f *HTTPForwarder) send(ctx context.Context, events []MetricEvent) error {
	body, err := json.Marshal(map[string][]MetricEvent{"events": events})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("service returned %s", resp.Status)
	}
	return nil
}
