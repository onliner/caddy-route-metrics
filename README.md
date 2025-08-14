# Caddy route metrics

[![CI](https://github.com/onliner/caddy-route-metrics/actions/workflows/ci.yml/badge.svg)](https://github.com/onliner/caddy-route-metrics/actions/workflows/ci.yml)

A simple Caddy HTTP middleware that records request duration metrics per route and exposes them to Prometheus.

## Installation

Build Caddy with this plugin using [xcaddy](https://github.com/caddyserver/xcaddy):

```bash
xcaddy build \
  --with github.com/onliner/caddy-route-metrics
```

## Usage

Enable `route_metrics` inside your Caddyfile.  
By default, the header name is `X-Route-Pattern`.

```caddy
{
  metrics
  admin localhost:2019
}

example.com {
  route_metrics {
    header X-Route-Pattern
  }

  handle_path /ok* {
    header {
      X-Route-Pattern "/users/{id}"
    }
    respond "ok"
  }

  handle_path /miss* {
    respond "miss"
  }
}
```

## Configuration options

The directive `route_metrics` supports the following option:

- **header &lt;name&gt;** *(optional)*  
  The HTTP header name from which to read the route pattern. This header must be set on the response by your application or middleware.  
  Defaults to `X-Route-Pattern`.

## Metrics

The middleware records one Prometheus histogram for request duration:

| Metric name                          | Type       | Labels                 |
|--------------------------------------|------------|------------------------|
| caddy_route_request_duration_seconds | Histogram  | `method`, `route`      |

### Details

- `method` — the HTTP method (e.g. GET, POST).  
- `route` — the route pattern taken from the configured header (e.g. `/users/{id}`).  

### Notes

- If the header is not present in the response, the request is skipped and no metric is recorded.  
