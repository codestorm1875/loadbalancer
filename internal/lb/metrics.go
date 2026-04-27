package lb

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type backendCounters struct {
	Requests atomic.Uint64
	Errors   atomic.Uint64
}

// Metrics captures basic operational counters.
type Metrics struct {
	TotalRequests  atomic.Uint64
	TotalErrors    atomic.Uint64
	RateLimited    atomic.Uint64
	ActiveRequests atomic.Int64

	mu       sync.RWMutex
	backends map[string]*backendCounters
}

func NewMetrics() *Metrics {
	return &Metrics{backends: make(map[string]*backendCounters)}
}

func (m *Metrics) backend(name string) *backendCounters {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.backends[name]; ok {
		return c
	}
	c := &backendCounters{}
	m.backends[name] = c
	return c
}

func (m *Metrics) IncBackendRequest(name string) {
	m.backend(name).Requests.Add(1)
}

func (m *Metrics) IncBackendError(name string) {
	m.backend(name).Errors.Add(1)
}

func (m *Metrics) Handler(backends []*Backend) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var b strings.Builder
		fmt.Fprintf(&b, "lb_requests_total %d\n", m.TotalRequests.Load())
		fmt.Fprintf(&b, "lb_errors_total %d\n", m.TotalErrors.Load())
		fmt.Fprintf(&b, "lb_ratelimited_total %d\n", m.RateLimited.Load())
		fmt.Fprintf(&b, "lb_active_requests %d\n", m.ActiveRequests.Load())

		sorted := make([]*Backend, 0, len(backends))
		sorted = append(sorted, backends...)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Name < sorted[j].Name
		})

		for _, be := range sorted {
			alive := 0
			if be.IsAlive() {
				alive = 1
			}
			fmt.Fprintf(&b, "lb_backend_up{name=\"%s\"} %d\n", be.Name, alive)
			fmt.Fprintf(&b, "lb_backend_active_connections{name=\"%s\"} %d\n", be.Name, be.ActiveConnections())
		}

		m.mu.RLock()
		names := make([]string, 0, len(m.backends))
		for n := range m.backends {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			c := m.backends[n]
			fmt.Fprintf(&b, "lb_backend_requests_total{name=\"%s\"} %d\n", n, c.Requests.Load())
			fmt.Fprintf(&b, "lb_backend_errors_total{name=\"%s\"} %d\n", n, c.Errors.Load())
		}
		m.mu.RUnlock()

		_, _ = w.Write([]byte(b.String()))
	})
}
