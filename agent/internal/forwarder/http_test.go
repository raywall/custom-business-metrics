package forwarder

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestHTTPForwarderSend(t *testing.T) {
	var accepted int
	forwarder := NewHTTPForwarder("http://service.local/v1/metrics", slog.Default())
	forwarder.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Events []MetricEvent `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		accepted = len(body.Events)
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Status:     "202 Accepted",
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})}

	err := forwarder.send(t.Context(), []MetricEvent{{
		Name:      "installments.processed",
		Kind:      "count",
		Value:     1,
		Timestamp: time.Now().UTC(),
	}})
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if accepted != 1 {
		t.Fatalf("expected 1 event, got %d", accepted)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
