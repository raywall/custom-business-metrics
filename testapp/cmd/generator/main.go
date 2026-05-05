package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"
)

type metricEvent struct {
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

// main sends synthetic business metrics to the UDP agent.
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	addr := env("AGENT_UDP_ADDR", "localhost:8125")
	rate := envInt("GENERATOR_RATE_PER_SECOND", 8)

	conn, err := net.Dial("udp", addr)
	if err != nil {
		logger.Error("udp dial failed", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	ticker := time.NewTicker(time.Second / time.Duration(rate))
	defer ticker.Stop()

	logger.Info("generator started", "addr", addr, "ratePerSecond", rate)
	for {
		select {
		case <-ticker.C:
			event := nextEvent()
			payload, _ := json.Marshal(event)
			if _, err := conn.Write(payload); err != nil {
				logger.Warn("metric send failed", "error", err)
			}
		case <-stop:
			logger.Info("generator stopped")
			return
		}
	}
}

func nextEvent() metricEvent {
	segments := []string{"EP", "OP", "INSS"}
	journeySteps := []string{"iniciador", "step-functions-baixa", "desconto-complementar", "ressarcimento", "conciliacao", "finalizacao"}
	results := []string{"baixa-realizada", "duplicidade", "evento-dados-invalidos", "ressarcimento-realizado", "aguardando-retentativa"}
	segment := segments[mathrand.Intn(len(segments))]
	journeyStep := journeySteps[mathrand.Intn(len(journeySteps))]
	processingCount := weightedProcessingCount()
	result := results[mathrand.Intn(len(results))]
	status := "success"
	name := "installments.processed"
	kind := "count"
	unit := "items"
	value := float64(mathrand.Intn(7) + 1)

	if result == "duplicidade" || result == "evento-dados-invalidos" {
		status = "error"
	}

	if status == "error" && mathrand.Float64() < 0.7 {
		name = "installments.error"
		value = float64(mathrand.Intn(3) + 1)
	} else if journeyStep == "step-functions-baixa" || result == "baixa-realizada" {
		name = "installments.settled"
	} else if journeyStep == "ressarcimento" || result == "ressarcimento-realizado" {
		name = "installments.refunded"
	} else if journeyStep == "finalizacao" {
		name = "installments.result"
	} else if mathrand.Float64() < 0.35 {
		name = "installments.amount"
		kind = "money"
		unit = "BRL"
		value = float64(50000+mathrand.Intn(900000)) / 100
	}

	traceID := newID("trace")
	correlationID := newID("corr")
	return metricEvent{
		Name:      name,
		Kind:      kind,
		Value:     value,
		Unit:      unit,
		Segment:   segment,
		Workflow:  "installment-lifecycle",
		Step:      journeyStep,
		Status:    status,
		Source:    "loan-core-simulator",
		Timestamp: time.Now().UTC(),
		Tags: map[string]string{
			"product":          "payroll-loan",
			"channel":          []string{"api", "batch", "partner"}[mathrand.Intn(3)],
			"etapa":            journeyStep,
			"processing_count": fmt.Sprintf("%d", processingCount),
			"result":           result,
			"correlation_id":   correlationID,
			"trace_id":         traceID,
			"parcela_id":       newID("parcela"),
		},
	}
}

func weightedProcessingCount() int {
	value := mathrand.Float64()
	switch {
	case value < 0.75:
		return 1
	case value < 0.93:
		return 2
	default:
		return 3 + mathrand.Intn(3)
	}
}

func newID(prefix string) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%x", prefix, b)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(env(key, ""))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
