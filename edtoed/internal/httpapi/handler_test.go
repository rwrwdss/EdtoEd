package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edtoed/internal/ml"
	"edtoed/internal/service"
)

type testStore struct{}

func (testStore) SaveLog(string, string, int, float32, float32) error { return nil }

func TestAnalyzeAndDocsRoutes(t *testing.T) {
	h := Handler{Analyzer: &service.Analyzer{Store: testStore{}, Predictor: ml.NewLinearPredictor()}}.Routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/analyze", strings.NewReader(`{"student_id":"s1","topic_id":"t1","clicks_count":10,"test_score":0.8,"deadline_delay":0}`))
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
