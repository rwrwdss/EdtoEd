package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	pb "edtoed/api/proto"
	"edtoed/internal/auth"
	"edtoed/internal/service"
	"edtoed/internal/telemetry"
)

type Handler struct {
	Analyzer *service.Analyzer
	Keys     *auth.Keys
	Metrics  *telemetry.Metrics
}

type analyzeRequest struct {
	CohortID      string  `json:"cohort_id"`
	WindowStart   int64   `json:"window_start"`
	WindowEnd     int64   `json:"window_end"`
	StudentID     string  `json:"student_id"`
	TopicID       string  `json:"topic_id"`
	ClicksCount   int32   `json:"clicks_count"`
	TestScore     float32 `json:"test_score"`
	DeadlineDelay float32 `json:"deadline_delay"`
}

type analyzeResponse struct {
	StudentID      string  `json:"student_id"`
	BurnoutRisk    float32 `json:"burnout_risk"`
	RequiresAction bool    `json:"requires_action"`
	ActionType     string  `json:"action_type"`
}

func (h Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", h.ready)
	mux.HandleFunc("GET /openapi.yaml", openAPI)
	mux.HandleFunc("GET /docs", swaggerUI)
	analyze := h.analyze
	if h.Metrics != nil {
		analyze = h.Metrics.HTTP(analyze)
		mux.Handle("GET /metrics", h.Metrics)
	}
	mux.HandleFunc("POST /v1/analyze", analyze)
	return withJSONHeaders(mux)
}

func health(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func (h Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if h.Analyzer == nil || h.Analyzer.Store == nil || h.Analyzer.Predictor == nil {
		http.Error(w, `{"status":"not_ready"}`, http.StatusServiceUnavailable)
		return
	}
	if health, ok := h.Analyzer.Predictor.(interface{ Ready(context.Context) bool }); ok && !health.Ready(ctx) {
		http.Error(w, "model unavailable", http.StatusServiceUnavailable)
		return
	}
	if health, ok := h.Analyzer.Store.(interface{ Ping(context.Context) error }); ok {
		if err := health.Ping(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h Handler) analyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || h.Analyzer == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ctx, ok := h.Keys.Authenticate(r.Context(), r.Header.Values("Authorization"))
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "valid bearer key required", http.StatusUnauthorized)
		return
	}
	var input analyzeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "expected one JSON object", http.StatusBadRequest)
		return
	}
	result, err := h.Analyzer.AnalyzeStudentMetrics(ctx, &pb.StudentActivityRequest{
		StudentId: input.StudentID, TopicId: input.TopicID, ClicksCount: input.ClicksCount,
		TestScore: input.TestScore, DeadlineDelay: input.DeadlineDelay,
		CohortId: input.CohortID, WindowStart: input.WindowStart, WindowEnd: input.WindowEnd,
	})
	if err != nil {
		statusCode := http.StatusInternalServerError
		if resultStatus := grpcCodeToHTTP(err); resultStatus != 0 {
			statusCode = resultStatus
		}
		http.Error(w, err.Error(), statusCode)
		return
	}
	writeJSON(w, http.StatusOK, analyzeResponse{result.StudentId, result.BurnoutRisk, result.RequiresAction, result.ActionType})
}

func withJSONHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
