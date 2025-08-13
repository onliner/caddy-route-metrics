package routemetrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/prometheus/client_golang/prometheus"
)

func durationCount(t *testing.T, reg *prometheus.Registry, method, route string) uint64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "caddy_route_request_duration_seconds" {
			continue
		}
		for _, m := range mf.GetMetric() {
			var gotMethod, gotRoute string
			for _, lp := range m.GetLabel() {
				switch lp.GetName() {
				case "method":
					gotMethod = lp.GetValue()
				case "route":
					gotRoute = lp.GetValue()
				}
			}
			if gotMethod == method && gotRoute == route && m.GetHistogram() != nil {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func newMiddlewareWithRegistry(header string) (*Metrics, *prometheus.Registry) {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Header: header,
		durs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "caddy",
			Subsystem: "route",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration (seconds)",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}
	reg.MustRegister(m.durs)
	return m, reg
}

func TestValidate_SetsDefaultHeader(t *testing.T) {
	m := &Metrics{}
	if err := m.Validate(); err != nil {
		t.Fatalf("validate error: %v", err)
	}
	if m.Header != "X-Route-Pattern" {
		t.Fatalf("default header not applied, got %q", m.Header)
	}
}

func TestUnmarshalCaddyfile_PositionalHeader(t *testing.T) {
	input := "route_metrics X-My-Route"
	d := caddyfile.NewTestDispenser(input)
	var m Metrics
	if err := m.UnmarshalCaddyfile(d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Header != "X-My-Route" {
		t.Fatalf("expected header X-My-Route, got %q", m.Header)
	}
}

func TestUnmarshalCaddyfile_BlockHeader(t *testing.T) {
	input := `
route_metrics {
  header X-Block-Route
}`
	d := caddyfile.NewTestDispenser(input)
	var m Metrics
	if err := m.UnmarshalCaddyfile(d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Header != "X-Block-Route" {
		t.Fatalf("expected header X-Block-Route, got %q", m.Header)
	}
}

func TestServeHTTP_RecordsAndStripsHeader(t *testing.T) {
	m, reg := newMiddlewareWithRegistry("X-Route-Pattern")

	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("X-Route-Pattern", "/users/{id}")
		time.Sleep(1 * time.Millisecond) // ensure non-zero duration
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.test/users/42", nil)
	rec := httptest.NewRecorder()

	if err := m.ServeHTTP(rec, req, next); err != nil {
		t.Fatalf("ServeHTTP returned error: %v", err)
	}

	if got := rec.Header().Get("X-Route-Pattern"); got != "" {
		t.Fatalf("expected header to be stripped, got %q", got)
	}

	if c := durationCount(t, reg, "GET", "/users/{id}"); c < 1 {
		t.Fatalf("expected histogram count >= 1 for (GET,/users/{id}), got %d", c)
	}
}

func TestServeHTTP_SkipsWhenHeaderMissing(t *testing.T) {
	m, reg := newMiddlewareWithRegistry("X-Route-Pattern")

	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		// no header set
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("nope"))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.test/unknown", nil)
	rec := httptest.NewRecorder()

	if err := m.ServeHTTP(rec, req, next); err != nil {
		t.Fatalf("ServeHTTP returned error: %v", err)
	}

	if c := durationCount(t, reg, "GET", "/users/{id}"); c != 0 {
		t.Fatalf("expected no histogram samples, got %d", c)
	}
}

func TestServeHTTP_ErrorPropagates_NoMetric(t *testing.T) {
	m, reg := newMiddlewareWithRegistry("X-Route-Pattern")

	wantErr := caddyhttp.Error(http.StatusTeapot, nil)
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("X-Route-Pattern", "/users/{id}")
		return wantErr
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.test/users/42", nil)
	rec := httptest.NewRecorder()

	err := m.ServeHTTP(rec, req, next)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if err != wantErr {
		t.Fatalf("unexpected error: %v", err)
	}

	if c := durationCount(t, reg, "GET", "/users/{id}"); c != 0 {
		t.Fatalf("expected no histogram samples on error, got %d", c)
	}
}
