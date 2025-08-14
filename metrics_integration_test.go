package routemetrics

import (
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/caddytest"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

func TestRespond(t *testing.T) {
	tester := caddytest.NewTester(t)
	cfg, err := os.ReadFile("testdata/Caddyfile")
	if err != nil {
		t.Fatalf("read Caddyfile: %v", err)
	}
	tester.InitServer(string(cfg), "caddyfile")

	resp, _ := tester.AssertGetResponse("http://localhost:9080/ok", 200, "hello from /ok")
	if v := resp.Header.Get("X-Route-Pattern"); v != "" {
		t.Fatalf("X-Route-Pattern header not cleared: %v", v)
	}

	time.Sleep(10 * time.Millisecond)

	tester.AssertGetResponse("http://localhost:9080/miss", 200, "hello from /miss")
	time.Sleep(10 * time.Millisecond)

	resp, err = http.Get("http://localhost:2999/metrics")
	if err != nil {
		t.Fatalf("failed to get metrics endpoint: %v", err)
	}

	mf, err := parseMF(resp.Body)
	if err != nil {
		t.Fatalf("failed to read metrics response body: %v", err)
	}

	fam := mf["caddy_route_request_duration_seconds"]

	if fam == nil {
		t.Fatalf("expected caddy_route_request_duration_seconds metric family, got nil")
	}

	routes := map[string]struct{}{}
	for _, m := range fam.Metric {
		for _, lp := range m.Label {
			if lp.GetName() == "route" {
				routes[lp.GetValue()] = struct{}{}
			}
		}
	}
	if len(routes) != 1 {
		t.Fatalf("expected exactly 1 route label, got %d: %#v", len(routes), routes)
	}
	if _, ok := routes["/users/{id}"]; !ok {
		t.Fatalf("expected only route=/users/{id}, got: %#v", routes)
	}
}

func parseMF(body io.ReadCloser) (map[string]*dto.MetricFamily, error) {
	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(body)
	if err != nil {
		return nil, err
	}

	return mf, nil
}
