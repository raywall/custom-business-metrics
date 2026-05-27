package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"custom-business-metrics/service/internal/application"
	"custom-business-metrics/service/internal/core"
)

// Server owns the HTTP routes for the service.
type Server struct {
	metrics    *application.MetricService
	dashboards *application.DashboardService
	config     *application.ConfigService
	logger     *slog.Logger
}

// NewServer wires HTTP handlers to application services.
func NewServer(metrics *application.MetricService, dashboards *application.DashboardService, config *application.ConfigService, logger *slog.Logger) *Server {
	return &Server{metrics: metrics, dashboards: dashboards, config: config, logger: logger}
}

// Handler builds the HTTP router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /v1/config", s.getConfig)
	mux.HandleFunc("PUT /v1/config", s.saveConfig)
	mux.HandleFunc("POST /v1/metrics", s.ingest)
	mux.HandleFunc("GET /v1/metrics/events", s.events)
	mux.HandleFunc("GET /v1/metrics/trace/{traceId}", s.eventsByTrace)
	mux.HandleFunc("GET /v1/metrics", s.summaries)
	mux.HandleFunc("GET /v1/metrics/series", s.series)
	mux.HandleFunc("GET /v1/dimensions", s.dimensions)
	mux.HandleFunc("GET /v1/dashboards", s.listDashboards)
	mux.HandleFunc("POST /v1/dashboards", s.saveDashboard)
	mux.HandleFunc("DELETE /v1/dashboards/{id}", s.deleteDashboard)
	return cors(mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.config.Get())
}

func (s *Server) saveConfig(w http.ResponseWriter, r *http.Request) {
	var req core.RuntimeConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.config.Save(req))
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Events []core.MetricEvent `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("events are required"))
		return
	}
	if err := s.metrics.Ingest(r.Context(), req.Events); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"accepted": len(req.Events)})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	result, err := s.metrics.Events(r.Context(), parseFilter(r), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) eventsByTrace(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	filter := parseFilter(r)
	if filter.Tags == nil {
		filter.Tags = map[string]string{}
	}
	filter.Tags["trace_id"] = strings.TrimSpace(r.PathValue("traceId"))
	result, err := s.metrics.Events(r.Context(), filter, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) summaries(w http.ResponseWriter, r *http.Request) {
	result, err := s.metrics.Summaries(r.Context(), parseFilter(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) series(w http.ResponseWriter, r *http.Request) {
	bucket := parseDuration(r.URL.Query().Get("bucket"), time.Minute)
	result, err := s.metrics.Series(r.Context(), parseFilter(r), bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) dimensions(w http.ResponseWriter, r *http.Request) {
	result, err := s.metrics.Dimensions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listDashboards(w http.ResponseWriter, r *http.Request) {
	result, err := s.dashboards.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) saveDashboard(w http.ResponseWriter, r *http.Request) {
	var dashboard core.Dashboard
	if err := json.NewDecoder(r.Body).Decode(&dashboard); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.dashboards.Save(r.Context(), dashboard)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) deleteDashboard(w http.ResponseWriter, r *http.Request) {
	if err := s.dashboards.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseFilter(r *http.Request) core.MetricFilter {
	query := r.URL.Query()
	filter := core.MetricFilter{
		Name:     query.Get("name"),
		Segment:  query.Get("segment"),
		Workflow: query.Get("workflow"),
		Step:     query.Get("step"),
		Status:   query.Get("status"),
		Source:   query.Get("source"),
		GroupBy:  query.Get("groupBy"),
		Tags:     parseTags(r),
		TagIn:    parseTagIn(r),
		From:     parseTime(query.Get("from")),
		To:       parseTime(query.Get("to")),
	}
	return filter
}

func parseTagIn(r *http.Request) map[string][]string {
	tags := map[string][]string{}
	for key, values := range r.URL.Query() {
		if strings.HasPrefix(key, "tagIn.") && len(values) > 0 {
			tagKey := strings.TrimSpace(strings.TrimPrefix(key, "tagIn."))
			if tagKey == "" {
				continue
			}
			for _, value := range strings.Split(values[0], ",") {
				value = strings.TrimSpace(value)
				if value != "" {
					tags[tagKey] = append(tags[tagKey], value)
				}
			}
		}
	}
	return tags
}

func parseTags(r *http.Request) map[string]string {
	tags := map[string]string{}
	for key, values := range r.URL.Query() {
		if strings.HasPrefix(key, "tag.") && len(values) > 0 {
			tagKey := strings.TrimSpace(strings.TrimPrefix(key, "tag."))
			tagValue := strings.TrimSpace(values[0])
			if tagKey != "" && tagValue != "" {
				tags[tagKey] = tagValue
			}
		}
	}
	for _, raw := range r.URL.Query()["tags"] {
		for _, pair := range strings.Split(raw, ",") {
			key, value, ok := strings.Cut(pair, ":")
			if !ok {
				key, value, ok = strings.Cut(pair, "=")
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if ok && key != "" && value != "" {
				tags[key] = value
			}
		}
	}
	if traceID := strings.TrimSpace(r.URL.Query().Get("trace_id")); traceID != "" {
		tags["trace_id"] = traceID
	}
	if spanID := strings.TrimSpace(r.URL.Query().Get("span_id")); spanID != "" {
		tags["span_id"] = spanID
	}
	return tags
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(unix, 0).UTC()
	}
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	if strings.HasSuffix(value, "m") || strings.HasSuffix(value, "s") || strings.HasSuffix(value, "h") {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
