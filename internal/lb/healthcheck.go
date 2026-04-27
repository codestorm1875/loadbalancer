package lb

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type HealthChecker struct {
	backends      []*Backend
	client        *http.Client
	interval      time.Duration
	failThreshold int
	passThreshold int
	logger        *log.Logger

	mu          sync.Mutex
	consecutive map[*Backend]int
}

func NewHealthChecker(cfg HealthCheckConfig, backends []*Backend, logger *log.Logger) (*HealthChecker, error) {
	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil {
		return nil, fmt.Errorf("parse health interval: %w", err)
	}
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("parse health timeout: %w", err)
	}
	return &HealthChecker{
		backends: backends,
		client: &http.Client{
			Timeout: timeout,
		},
		interval:      interval,
		failThreshold: cfg.FailThreshold,
		passThreshold: cfg.PassThreshold,
		logger:        logger,
		consecutive:   make(map[*Backend]int),
	}, nil
}

func (h *HealthChecker) Start(ctx context.Context) {
	h.runOnce(ctx)
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.runOnce(ctx)
		}
	}
}

func (h *HealthChecker) runOnce(ctx context.Context) {
	for _, backend := range h.backends {
		h.checkBackend(ctx, backend)
	}
}

func (h *HealthChecker) checkBackend(ctx context.Context, backend *Backend) {
	path := backend.HealthPath
	if path == "" {
		path = "/health"
	}

	endpoint := backend.URL.String()
	if path[0] == '/' {
		endpoint += path
	} else {
		endpoint += "/" + path
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		h.markFailure(backend)
		return
	}
	resp, err := h.client.Do(req)
	if err != nil {
		h.markFailure(backend)
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		h.markSuccess(backend)
		return
	}
	h.markFailure(backend)
}

func (h *HealthChecker) markSuccess(backend *Backend) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.consecutive[backend] < 0 {
		h.consecutive[backend] = 0
	}
	h.consecutive[backend]++
	if !backend.IsAlive() && h.consecutive[backend] >= h.passThreshold {
		backend.SetAlive(true)
		h.logger.Printf("backend recovered: %s", backend.Name)
	}
}

func (h *HealthChecker) markFailure(backend *Backend) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.consecutive[backend] > 0 {
		h.consecutive[backend] = 0
	}
	h.consecutive[backend]--
	if backend.IsAlive() && -h.consecutive[backend] >= h.failThreshold {
		backend.SetAlive(false)
		h.logger.Printf("backend marked unhealthy: %s", backend.Name)
	}
}
