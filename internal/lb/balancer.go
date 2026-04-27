package lb

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"
)

type LoadBalancer struct {
	backends []*Backend
	strategy Strategy
	metrics  *Metrics
	logger   *log.Logger

	proxies map[*Backend]*httputil.ReverseProxy
	mu      sync.RWMutex
}

func New(cfg *Config, logger *log.Logger) (*LoadBalancer, *HealthChecker, error) {
	strategy, err := NewStrategy(cfg.Strategy)
	if err != nil {
		return nil, nil, err
	}

	backends := make([]*Backend, 0, len(cfg.Backends))
	for _, bc := range cfg.Backends {
		name := bc.Name
		if name == "" {
			name = bc.URL
		}
		backend, err := NewBackend(name, bc.URL, bc.Weight, bc.HealthPath)
		if err != nil {
			return nil, nil, err
		}
		backends = append(backends, backend)
	}

	lb := &LoadBalancer{
		backends: backends,
		strategy: strategy,
		metrics:  NewMetrics(),
		logger:   logger,
		proxies:  make(map[*Backend]*httputil.ReverseProxy),
	}
	lb.initProxies()

	hc, err := NewHealthChecker(cfg.HealthCheck, backends, logger)
	if err != nil {
		return nil, nil, err
	}

	return lb, hc, nil
}

func (lb *LoadBalancer) initProxies() {
	for _, backend := range lb.backends {
		b := backend
		target := backend.URL
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			b.SetAlive(false)
			lb.metrics.TotalErrors.Add(1)
			lb.metrics.IncBackendError(b.Name)
			lb.logger.Printf("upstream error backend=%s err=%v", b.Name, err)
			http.Error(w, "upstream error", http.StatusBadGateway)
		}
		lb.proxies[backend] = proxy
	}
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	lb.metrics.TotalRequests.Add(1)
	lb.metrics.ActiveRequests.Add(1)
	defer lb.metrics.ActiveRequests.Add(-1)

	backend := lb.nextBackend()
	if backend == nil {
		lb.metrics.TotalErrors.Add(1)
		http.Error(w, "no healthy backends", http.StatusServiceUnavailable)
		lb.logger.Printf("method=%s path=%s status=%d duration_ms=%d backend=none", r.Method, r.URL.Path, http.StatusServiceUnavailable, time.Since(start).Milliseconds())
		return
	}

	backend.IncActive()
	defer backend.DecActive()
	lb.metrics.IncBackendRequest(backend.Name)

	proxy := lb.proxies[backend]
	rw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
	rw.Header().Set("X-Backend", backend.Name)
	proxy.ServeHTTP(rw, r)

	if rw.code >= http.StatusInternalServerError {
		lb.metrics.TotalErrors.Add(1)
		lb.metrics.IncBackendError(backend.Name)
	}
	lb.logger.Printf("method=%s path=%s status=%d duration_ms=%d backend=%s strategy=%s", r.Method, r.URL.Path, rw.code, time.Since(start).Milliseconds(), backend.Name, lb.strategy.Name())
}

func (lb *LoadBalancer) nextBackend() *Backend {
	healthy := make([]*Backend, 0, len(lb.backends))
	for _, b := range lb.backends {
		if b.IsAlive() {
			healthy = append(healthy, b)
		}
	}
	return lb.strategy.Next(healthy)
}

func (lb *LoadBalancer) Metrics() *Metrics {
	return lb.metrics
}

func (lb *LoadBalancer) Backends() []*Backend {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	out := make([]*Backend, len(lb.backends))
	copy(out, lb.backends)
	return out
}

func (lb *LoadBalancer) StartHealthChecks(ctx context.Context, checker *HealthChecker) {
	go checker.Start(ctx)
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}
