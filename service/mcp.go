// Package service provides embeddable integrations for Custom Business Metrics.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MCPConfig configures the read-only analytics MCP server.
type MCPConfig struct {
	MetricsEndpoint string
	APIKey          string
	ServerAPIKey    string
	HTTPClient      *http.Client
}

// MCPServer exposes execution and metrics data to LLM clients.
type MCPServer struct {
	config MCPConfig
}

// NewMCPServer creates a read-only MCP analytics server.
func NewMCPServer(config MCPConfig) (*MCPServer, error) {
	if strings.TrimSpace(config.MetricsEndpoint) == "" {
		return nil, fmt.Errorf("metrics endpoint is required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &MCPServer{config: config}, nil
}

// Handler returns a JSON-RPC MCP HTTP handler.
func (s *MCPServer) Handler() http.Handler {
	return http.HandlerFunc(s.handle)
}

func (s *MCPServer) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.config.ServerAPIKey != "" && r.Header.Get("X-API-Key") != s.config.ServerAPIKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeMCPError(w, request.ID, -32700, err.Error())
		return
	}
	if request.Method == "tools/list" {
		writeMCPResult(w, request.ID, map[string]any{"tools": []map[string]any{
			{"name": "find_processes", "description": "Busca execucoes por filtros operacionais.", "inputSchema": objectSchema()},
			{"name": "get_execution_by_correlation", "description": "Recupera eventos de uma execucao por correlation_id.", "inputSchema": objectSchema()},
			{"name": "get_execution_by_trace", "description": "Recupera eventos tecnicos por trace_id.", "inputSchema": objectSchema()},
			{"name": "get_workflow_summary", "description": "Recupera metricas agregadas por workflow e periodo.", "inputSchema": objectSchema()},
		}})
		return
	}
	if request.Method != "tools/call" {
		writeMCPError(w, request.ID, -32601, "method not found")
		return
	}
	result, err := s.call(r.Context(), request.Params.Name, request.Params.Arguments)
	if err != nil {
		writeMCPError(w, request.ID, -32000, err.Error())
		return
	}
	writeMCPResult(w, request.ID, result)
}

func (s *MCPServer) call(ctx context.Context, name string, args map[string]any) (any, error) {
	values := url.Values{}
	for key, value := range args {
		if value != nil {
			values.Set(key, fmt.Sprint(value))
		}
	}
	path := "/v1/metrics/events"
	switch name {
	case "get_execution_by_correlation":
		values.Set("correlation_id", fmt.Sprint(args["correlation_id"]))
	case "get_execution_by_trace":
		traceID := url.PathEscape(fmt.Sprint(args["trace_id"]))
		path = "/v1/metrics/trace/" + traceID
	case "get_workflow_summary":
		path = "/v1/metrics"
	case "find_processes":
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	endpoint := strings.TrimRight(s.config.MetricsEndpoint, "/") + path
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if s.config.APIKey != "" {
		req.Header.Set("X-API-Key", s.config.APIKey)
	}
	resp, err := s.config.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("metrics API returned %s", resp.Status)
	}
	var result any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func objectSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": true}
}

func writeMCPResult(w http.ResponseWriter, id, result any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func writeMCPError(w http.ResponseWriter, id any, code int, message string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
}
