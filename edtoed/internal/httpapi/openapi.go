package httpapi

import (
	"net/http"

	"google.golang.org/grpc/status"
)

const openAPISpec = `openapi: 3.0.3
info:
  title: EdtoEd Analytics API
  version: 1.0.0
  description: Universal student-risk scoring API for LMS integrations.
servers:
  - url: /
paths:
  /healthz:
    get:
      summary: Liveness probe
      responses:
        '200': {description: Service is alive}
  /readyz:
    get:
      summary: Readiness probe
      responses:
        '200': {description: Service is ready}
        '503': {description: Service is not ready}
  /v1/analyze:
    post:
      summary: Score student activity
      operationId: analyzeStudentMetrics
      security:
        - platformKey: []
      requestBody:
        required: true
        content:
          application/json:
            schema: {$ref: '#/components/schemas/StudentActivityRequest'}
      responses:
        '200':
          description: Risk score
          content:
            application/json:
              schema: {$ref: '#/components/schemas/AnalyticsResponse'}
        '409': {description: Snapshot saved but cohort needs two students and positive median}
        '400': {description: Invalid request}
        '401': {description: Missing or invalid platform bearer key}
        '408': {description: Request canceled}
        '429': {description: Organization concurrency limit reached before snapshot storage}
        '503': {description: Model or dependency unavailable}
        '504': {description: Analysis deadline exceeded; the snapshot may already be saved}
components:
  securitySchemes:
    platformKey:
      type: http
      scheme: bearer
      description: Platform API key issued by the EdtoEd operator.
  schemas:
    StudentActivityRequest:
      type: object
      required: [cohort_id, window_start, window_end, student_id, topic_id, clicks_count, test_score, deadline_delay]
      properties:
        cohort_id: {type: string, example: lmsA_group1}
        window_start: {type: integer, format: int64, example: 1704067200, description: Inclusive Unix seconds}
        window_end: {type: integer, format: int64, example: 1706486400, description: Exclusive Unix seconds; completed window}
        student_id: {type: string, example: student_hash_123}
        topic_id: {type: string, example: algebra_01}
        clicks_count: {type: integer, format: int32, minimum: 0, example: 24}
        test_score: {type: number, format: float, minimum: 0, maximum: 1, example: 0.72}
        deadline_delay: {type: number, format: float, example: 1.5}
    AnalyticsResponse:
      type: object
      properties:
        student_id: {type: string}
        burnout_risk: {type: number, format: float, minimum: 0, maximum: 1}
        requires_action: {type: boolean}
        action_type: {type: string, enum: [KEEP_TRACK, OFFER_HINT]}
`

func openAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write([]byte(openAPISpec))
}

func swaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><title>EdtoEd API</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script><script>window.ui=SwaggerUIBundle({url:'/openapi.yaml',dom_id:'#swagger-ui',deepLinking:true});</script></body></html>`))
}

func grpcCodeToHTTP(err error) int {
	switch status.Code(err).String() {
	case "InvalidArgument":
		return http.StatusBadRequest
	case "Unauthenticated":
		return http.StatusUnauthorized
	case "ResourceExhausted":
		return http.StatusTooManyRequests
	case "FailedPrecondition":
		return http.StatusConflict
	case "Unavailable":
		return http.StatusServiceUnavailable
	case "Canceled":
		return http.StatusRequestTimeout
	case "DeadlineExceeded":
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}
