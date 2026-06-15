package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPListsAnalyticsTools(t *testing.T) {
	server, err := NewMCPServer(MCPConfig{MetricsEndpoint: "http://metrics"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("get_execution_by_correlation")) {
		t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
	}
}
