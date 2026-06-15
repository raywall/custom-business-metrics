// Package agent provides an embeddable asynchronous metrics agent.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Config controls batching and delivery to custom-business-metrics service.
type Config struct {
	ServiceEndpoint string
	BatchSize       int
	BufferSize      int
	FlushInterval   time.Duration
	HTTPClient      *http.Client
	Logger          *slog.Logger
}

// Agent asynchronously buffers and sends metric events.
type Agent struct {
	config Config
	events chan any
	done   chan struct{}
	once   sync.Once
}

// New creates and starts an embeddable metrics agent.
func New(config Config) (*Agent, error) {
	if config.ServiceEndpoint == "" {
		return nil, fmt.Errorf("metrics service endpoint is required")
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 25
	}
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = time.Second
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	return &Agent{config: config, events: make(chan any, config.BufferSize), done: make(chan struct{})}, nil
}

// Run processes buffered events until ctx is cancelled or Close is called.
func (a *Agent) Run(ctx context.Context) error {
	ticker := time.NewTicker(a.config.FlushInterval)
	defer ticker.Stop()
	batch := make([]any, 0, a.config.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := a.send(ctx, batch); err != nil {
			a.config.Logger.Warn("metric batch failed", "count", len(batch), "error", err)
			return
		}
		batch = batch[:0]
	}
	drainAndFlush := func() {
		for {
			select {
			case event := <-a.events:
				batch = append(batch, event)
			default:
				flush()
				return
			}
		}
	}
	for {
		select {
		case event := <-a.events:
			batch = append(batch, event)
			if len(batch) > a.config.BufferSize {
				a.config.Logger.Warn("metric retry buffer full; dropping oldest event")
				batch = batch[1:]
			}
			if len(batch) >= a.config.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-a.done:
			drainAndFlush()
			return nil
		case <-ctx.Done():
			drainAndFlush()
			return ctx.Err()
		}
	}
}

// Emit queues a serializable metric event without blocking the workflow.
func (a *Agent) Emit(_ context.Context, event any) error {
	select {
	case a.events <- event:
		return nil
	default:
		return fmt.Errorf("metrics agent buffer is full")
	}
}

// Close requests a graceful final flush.
func (a *Agent) Close() {
	a.once.Do(func() { close(a.done) })
}

func (a *Agent) send(ctx context.Context, events []any) error {
	body, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.ServiceEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.config.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("metrics service returned %s", resp.Status)
	}
	return nil
}
