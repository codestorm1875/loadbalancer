A production-style HTTP load balancer in Go with strategy switching, health checks, metrics, and integrated request rate limiting.

Current stable version: `v1.0.0`

License: MIT (redistributable)

## Features

- Multiple balancing strategies:
  - `round-robin`
  - `least-connections`
  - `weighted` (smooth weighted round robin)
- Active health checks that mark unhealthy backends out of rotation
- YAML/JSON config loading
- Request logging
- Basic Prometheus-style metrics endpoint at `/metrics`
- Optional integration with `github.com/codestorm1875/ratelimiter` as middleware

## Architecture

```mermaid
flowchart LR
  C[Client] --> RL[Rate Limiter Middleware]
  RL --> LB[Load Balancer Handler]
  LB --> S{Strategy Engine}
  S --> A[Backend A]
  S --> B[Backend B]
  S --> N[Backend N]
  HC[Health Checker] --> A
  HC --> B
  HC --> N
  LB --> M[Metrics Registry]
  M --> P[/metrics]
```

Request path summary:

1. Incoming traffic enters optional rate limiting middleware.
2. The balancer selects from healthy backends using configured strategy.
3. Active health checks continuously mark backends in or out of rotation.
4. Request and backend counters are exposed at `/metrics`.

## Run

```bash
make run
```

For local development, start two mock backends in separate terminals first:

```bash
make backend-a
make backend-b
```

Then start the load balancer:

```bash
make run
```

Note: `config.example.yaml` has rate limiting disabled by default for smoother local smoke tests.
Enable `rate_limit.enabled: true` when you want to validate throttling behavior.

## Config

Copy `config.example.yaml` and adjust backend URLs.

The same config fields work for JSON and YAML files.

## Endpoints

- `/` proxied to selected upstream backend
- `/metrics` exposes counters and per-backend state
- `/health` local balancer health endpoint

## Testing

Standard workflow:

```bash
make check
```

Useful focused commands:

```bash
make test
make test-integration
make bench
```

Direct Go commands remain available:

```bash
go mod tidy
go test ./...
go test -run TestIntegrationFailoverAndRecovery ./internal/lb
```

## Metrics Provided

- `lb_requests_total`
- `lb_errors_total`
- `lb_ratelimited_total`
- `lb_active_requests`
- `lb_backend_up{name="..."}`
- `lb_backend_active_connections{name="..."}`
- `lb_backend_requests_total{name="..."}`
- `lb_backend_errors_total{name="..."}`

## Portfolio Story

This project demonstrates a layered architecture:

1. Traffic shaping at the edge with strategy-based load distribution
2. Automatic backend lifecycle management via active health checks
3. Abuse control with fairness-aware rate limiting middleware
4. Observability through structured logs and scrape-friendly metrics

## Versioning And Releases

- Semantic versioning is used for stable releases.
- The current release tag is `v1.0.0`.
- Release notes are tracked in `CHANGELOG.md`.
