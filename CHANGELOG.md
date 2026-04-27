# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project follows Semantic Versioning.

## [1.0.0] - 2026-04-27

### Added
- Multi-strategy load balancing: round-robin, least-connections, weighted
- Active health checks with automatic backend removal and recovery
- YAML/JSON configuration support
- Request logging and Prometheus-style `/metrics` endpoint
- Middleware integration with `github.com/codestorm1875/ratelimiter`
- Integration tests for failover/recovery and least-connections concurrent behavior
- Makefile for run/test/bench/check workflows
- MIT License for broad redistribution
