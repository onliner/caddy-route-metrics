package routemetrics

import (
	"net/http"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/felixge/httpsnoop"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	Header string `json:"header"`

	durs *prometheus.HistogramVec
}

var (
	_ caddy.Provisioner           = (*Metrics)(nil)
	_ caddy.Validator             = (*Metrics)(nil)
	_ caddyhttp.MiddlewareHandler = (*Metrics)(nil)
	_ caddyfile.Unmarshaler       = (*Metrics)(nil)
)

func init() {
	caddy.RegisterModule(Metrics{})
	httpcaddyfile.RegisterHandlerDirective("route_metrics", parseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder("route_metrics", httpcaddyfile.After, "encode")
}

func (Metrics) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.route_metrics",
		New: func() caddy.Module { return new(Metrics) },
	}
}

func (m *Metrics) Provision(ctx caddy.Context) error {
	labels := []string{"method", "route"}
	buckets := prometheus.DefBuckets

	m.durs = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "caddy",
		Subsystem: "route",
		Name:      "request_duration_seconds",
		Help:      "HTTP request duration (seconds)",
		Buckets:   buckets,
	}, labels)

	ctx.GetMetricsRegistry().MustRegister(m.durs)
	return nil
}

func (m *Metrics) Validate() error {
	if m.Header == "" {
		m.Header = "X-Route-Pattern"
	}
	return nil
}

func (m *Metrics) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if d.NextArg() {
			m.Header = d.Val()
			if d.NextArg() {
				return d.ArgErr()
			}
		}

		for d.NextBlock(0) {
			switch d.Val() {
			case "header":
				var hdr string
				if !d.Args(&hdr) {
					return d.ArgErr()
				}
				m.Header = hdr
			default:
				return d.Errf("unrecognized subdirective: %s", d.Val())
			}
		}
	}
	return nil
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	route := ""

	hooks := httpsnoop.Hooks{
		WriteHeader: func(orig httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return func(code int) {
				if v := w.Header().Get(m.Header); v != "" {
					route = v
					w.Header().Del(m.Header)
				}
				orig(code)
			}
		},
	}

	var err error
	wrapped := httpsnoop.Wrap(w, hooks)
	metrics := httpsnoop.CaptureMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err = next.ServeHTTP(w, r)
	}), wrapped, r)

	if route == "" {
		return err
	}

	m.durs.WithLabelValues(r.Method, route).Observe(metrics.Duration.Seconds())

	return err
}

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	m := new(Metrics)
	err := m.UnmarshalCaddyfile(h.Dispenser)

	return m, err
}
