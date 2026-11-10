package httpapi

import (
	"context"
	pb "edtoed/api/proto"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edtoed/internal/features"
	"edtoed/internal/service"
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

func (testStore) SnapshotMedian(context.Context, *pb.StudentActivityRequest) (float64, int, error) {
	return 5, 2, nil
}

func TestAnalyzeAndDocsRoutes(t *testing.T) {
	h := Handler{Analyzer: &service.Analyzer{Store: testStore{}, Predictor: testPredictor{}}}.Routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(`{"cohort_id":"c","window_start":1,"window_end":2,"student_id":"s1","topic_id":"t1","clicks_count":10,"test_score":0.8,"deadline_delay":0}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
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
	h := Handler{Analyzer: &service.Analyzer{Store: testStore{}, Predictor: testPredictor{}}}.Routes()
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(`{} {}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("multiple JSON values: %d", res.Code)
	}
}

func TestAnalyzeReportsInferenceTimeout(t *testing.T) {
	h := Handler{Analyzer: &service.Analyzer{Store: testStore{}, Predictor: timedOutPredictor{}}}.Routes()
	body := `{"cohort_id":"c","window_start":1,"window_end":2,"student_id":"s","topic_id":"t","clicks_count":10,"test_score":0.8,"deadline_delay":0}`
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(body)))
	if res.Code != http.StatusGatewayTimeout {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
}
