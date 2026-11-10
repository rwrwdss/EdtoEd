package httpapi

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pb "edtoed/api/proto"
	"edtoed/internal/auth"
	"edtoed/internal/features"
	"edtoed/internal/service"
	"edtoed/internal/telemetry"
)

type testStore struct{}
type testPredictor struct{}
type unhealthyPredictor struct{ testPredictor }
type timedOutPredictor struct{}

func (timedOutPredictor) Predict(ctx context.Context, _ features.Vector) (float32, error) {
	if _, ok := ctx.Deadline(); !ok {
		return 0, errors.New("analysis context has no deadline")
	}
	return 0, context.DeadlineExceeded
}

func (unhealthyPredictor) Ready(context.Context) bool { return false }

type unhealthyStore struct{ testStore }

func (unhealthyStore) Ping(context.Context) error { return errors.New("database down") }

func (testPredictor) Predict(context.Context, features.Vector) (float32, error) { return 0.3, nil }

func (testStore) SnapshotMedian(context.Context, string, *pb.StudentActivityRequest) (float64, int, error) {
	return 5, 2, nil
}

func analysisHandler(t *testing.T, analyzer *service.Analyzer) (http.Handler, string) {
	t.Helper()
	token := strings.Repeat("a", 64)
	registry := fmt.Sprintf(`[{"organization_id":"platform","sha256":"%x"}]`, sha256.Sum256([]byte(token)))
	keys, err := auth.Load(strings.NewReader(registry))
	if err != nil {
		t.Fatal(err)
	}
	metrics := telemetry.New()
	analyzer.Metrics = metrics
	return Handler{Analyzer: analyzer, Keys: keys, Metrics: metrics}.Routes(), "Bearer " + token
}

func TestAnalyzeAndDocsRoutes(t *testing.T) {
	h, authorization := analysisHandler(t, &service.Analyzer{Store: testStore{}, Predictor: testPredictor{}})

	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(`{"cohort_id":"c","window_start":1,"window_end":2,"student_id":"s1","topic_id":"t1","clicks_count":10,"test_score":0.8,"deadline_delay":0}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("missing credentials: status %d", res.Code)
	}
	req.Header.Set("Authorization", authorization)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"burnout_risk"`) {
		t.Fatalf("unexpected analyze response: %d %s", res.Code, res.Body.String())
	}

	for _, path := range []string{"/healthz", "/readyz", "/openapi.yaml", "/docs"} {
		res = httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, res.Code)
		}
	}

	res = httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, sample := range []string{
		`edtoed_analysis_requests_total{transport="rest",code="401"} 1`,
		`edtoed_analysis_requests_total{transport="rest",code="200"} 1`,
		`edtoed_analysis_in_flight{transport="rest"} 0`,
		`edtoed_analysis_duration_seconds_count{transport="rest"} 2`,
		`edtoed_analysis_stage_duration_seconds_count{stage="storage"} 1`,
		`edtoed_analysis_stage_duration_seconds_count{stage="inference"} 1`,
	} {
		if !strings.Contains(res.Body.String(), sample+"\n") {
			t.Errorf("missing sample %q", sample)
		}
	}
	if strings.Contains(res.Body.String(), authorization) {
		t.Fatal("metrics exposed the authorization header")
	}
}

func TestReadinessAndJSONBoundary(t *testing.T) {
	for _, a := range []*service.Analyzer{
		{Store: testStore{}, Predictor: unhealthyPredictor{}},
		{Store: unhealthyStore{}, Predictor: testPredictor{}},
	} {
		res := httptest.NewRecorder()
		Handler{Analyzer: a}.Routes().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if res.Code != http.StatusServiceUnavailable {
			t.Fatalf("readiness: %d", res.Code)
		}
	}
	h, authorization := analysisHandler(t, &service.Analyzer{Store: testStore{}, Predictor: testPredictor{}})
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(`{} {}`))
	req.Header.Set("Authorization", authorization)
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("multiple JSON values: %d", res.Code)
	}
}

func TestAnalyzeReportsInferenceTimeout(t *testing.T) {
	h, authorization := analysisHandler(t, &service.Analyzer{Store: testStore{}, Predictor: timedOutPredictor{}})
	body := `{"cohort_id":"c","window_start":1,"window_end":2,"student_id":"s","topic_id":"t","clicks_count":10,"test_score":0.8,"deadline_delay":0}`
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(body))
	req.Header.Set("Authorization", authorization)
	h.ServeHTTP(res, req)
	if res.Code != http.StatusGatewayTimeout {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
}
