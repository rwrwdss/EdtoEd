package telemetry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

var latencyBounds = [...]float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

type histogram struct {
	buckets [len(latencyBounds)]uint64
	count   uint64
	sum     float64
}

func (h *histogram) observe(duration time.Duration) {
	seconds := duration.Seconds()
	h.count++
	h.sum += seconds
	for i, bound := range latencyBounds {
		if seconds <= bound {
			h.buckets[i]++
		}
	}
}

type transport struct {
	inFlight int
	byCode   map[string]uint64
	latency  histogram
}

type Metrics struct {
	mu        sync.Mutex
	rest      transport
	grpc      transport
	storage   histogram
	inference histogram
}

func New() *Metrics {
	return &Metrics{
		rest: transport{byCode: make(map[string]uint64)},
		grpc: transport{byCode: make(map[string]uint64)},
	}
}

func (m *Metrics) start(t *transport) func(string) {
	started := time.Now()
	m.mu.Lock()
	t.inFlight++
	m.mu.Unlock()
	return func(code string) {
		elapsed := time.Since(started)
		m.mu.Lock()
		defer m.mu.Unlock()
		t.inFlight--
		t.byCode[code]++
		t.latency.observe(elapsed)
	}
}

func (m *Metrics) ObserveStorage(elapsed time.Duration) {
	if m != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.storage.observe(elapsed)
	}
}

func (m *Metrics) ObserveInference(elapsed time.Duration) {
	if m != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.inference.observe(elapsed)
	}
}

func (m *Metrics) HTTP(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		record := &statusWriter{ResponseWriter: w}
		finish := m.start(&m.rest)
		defer func() {
			code := record.code
			if code == 0 {
				code = http.StatusInternalServerError
			}
			finish(strconv.Itoa(code))
		}()
		next(record, r)
	}
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.code == 0 && code >= 200 {
		w.code = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (m *Metrics) Unary(ctx context.Context, request any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (response any, err error) {
	finish := m.start(&m.grpc)
	code := "Internal"
	defer func() { finish(code) }()
	response, err = next(ctx, request)
	code = status.Code(err).String()
	return response, err
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = io.WriteString(w, m.render())
}

func (m *Metrics) render() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var output strings.Builder
	output.WriteString("# HELP edtoed_analysis_requests_total Completed analysis requests by transport and response code.\n")
	output.WriteString("# TYPE edtoed_analysis_requests_total counter\n")
	for _, entry := range m.transports() {
		codes := make([]string, 0, len(entry.data.byCode))
		for code := range entry.data.byCode {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		for _, code := range codes {
			fmt.Fprintf(&output, "edtoed_analysis_requests_total{transport=%q,code=%q} %d\n", entry.name, code, entry.data.byCode[code])
		}
	}
	output.WriteString("# HELP edtoed_analysis_in_flight Analysis requests currently being handled.\n")
	output.WriteString("# TYPE edtoed_analysis_in_flight gauge\n")
	for _, entry := range m.transports() {
		fmt.Fprintf(&output, "edtoed_analysis_in_flight{transport=%q} %d\n", entry.name, entry.data.inFlight)
	}
	output.WriteString("# HELP edtoed_analysis_duration_seconds Analysis request duration at the server.\n")
	output.WriteString("# TYPE edtoed_analysis_duration_seconds histogram\n")
	for _, entry := range m.transports() {
		entry.data.latency.write(&output, "edtoed_analysis_duration_seconds", "transport", entry.name)
	}
	output.WriteString("# HELP edtoed_analysis_stage_duration_seconds Time spent in storage and inference, including waits.\n")
	output.WriteString("# TYPE edtoed_analysis_stage_duration_seconds histogram\n")
	m.storage.write(&output, "edtoed_analysis_stage_duration_seconds", "stage", "storage")
	m.inference.write(&output, "edtoed_analysis_stage_duration_seconds", "stage", "inference")
	return output.String()
}

type namedTransport struct {
	name string
	data *transport
}

func (m *Metrics) transports() [2]namedTransport {
	return [2]namedTransport{{"rest", &m.rest}, {"grpc", &m.grpc}}
}

func (h *histogram) write(w io.Writer, name, label, value string) {
	for i, bound := range latencyBounds {
		fmt.Fprintf(w, "%s_bucket{%s=%q,le=%q} %d\n", name, label, value, strconv.FormatFloat(bound, 'f', -1, 64), h.buckets[i])
	}
	fmt.Fprintf(w, "%s_bucket{%s=%q,le=\"+Inf\"} %d\n", name, label, value, h.count)
	fmt.Fprintf(w, "%s_sum{%s=%q} %g\n", name, label, value, h.sum)
	fmt.Fprintf(w, "%s_count{%s=%q} %d\n", name, label, value, h.count)
}
