package collector

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"time"
)

// MetricEvent mirrors the public JSON contract accepted by the service.
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

// UDPCollector receives JSON metric packets.
type UDPCollector struct {
	addr   string
	logger *slog.Logger
}

// NewUDPCollector creates a UDP collector bound to addr.
func NewUDPCollector(addr string, logger *slog.Logger) *UDPCollector {
	return &UDPCollector{addr: addr, logger: logger}
}

// Run starts the UDP loop and publishes decoded metrics to out.
func (c *UDPCollector) Run(ctx context.Context, out chan<- MetricEvent) error {
	pc, err := net.ListenPacket("udp", c.addr)
	if err != nil {
		return err
	}
	defer pc.Close()

	c.logger.Info("udp collector started", "addr", c.addr)
	go func() {
		<-ctx.Done()
		_ = pc.Close()
	}()

	buffer := make([]byte, 64*1024)
	for {
		n, _, err := pc.ReadFrom(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Warn("udp read failed", "error", err)
			continue
		}
		var event MetricEvent
		if err := json.Unmarshal(buffer[:n], &event); err != nil {
			c.logger.Warn("invalid metric packet", "error", err)
			continue
		}
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now().UTC()
		}
		select {
		case out <- event:
		case <-ctx.Done():
			return nil
		}
	}
}
