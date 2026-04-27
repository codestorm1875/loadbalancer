package lb

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func eventually(t *testing.T, timeout time.Duration, tick time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(tick)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestIntegrationFailoverAndRecovery(t *testing.T) {
	var aHealthy atomic.Bool
	aHealthy.Store(true)

	backendA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			if !aHealthy.Load() {
				http.Error(w, "down", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("from-a"))
		}
	}))
	defer backendA.Close()

	backendB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("from-b"))
	}))
	defer backendB.Close()

	cfg := &Config{
		Strategy: "round-robin",
		HealthCheck: HealthCheckConfig{
			Interval:      "20ms",
			Timeout:       "100ms",
			FailThreshold: 1,
			PassThreshold: 1,
		},
		Backends: []BackendConfig{
			{Name: "a", URL: backendA.URL, Weight: 1, HealthPath: "/health"},
			{Name: "b", URL: backendB.URL, Weight: 1, HealthPath: "/health"},
		},
	}

	balancer, checker, err := New(cfg, log.New(testWriter{t: t}, "", 0))
	if err != nil {
		t.Fatalf("new balancer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	balancer.StartHealthChecks(ctx, checker)

	proxy := httptest.NewServer(balancer)
	defer proxy.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}

	// Warm-up confirms both backends receive traffic before failover.
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		resp, err := client.Get(proxy.URL + "/")
		if err != nil {
			t.Fatalf("request before failover: %v", err)
		}
		_ = resp.Body.Close()
		seen[resp.Header.Get("X-Backend")] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("expected traffic to hit both backends before failover, seen=%v", seen)
	}

	// Trigger backend A health failure and wait for it to be removed.
	aHealthy.Store(false)
	var a *Backend
	for _, be := range balancer.Backends() {
		if be.Name == "a" {
			a = be
			break
		}
	}
	if a == nil {
		t.Fatal("backend a not found")
	}

	eventually(t, time.Second, 20*time.Millisecond, func() bool {
		return !a.IsAlive()
	})

	for i := 0; i < 6; i++ {
		resp, err := client.Get(proxy.URL + "/")
		if err != nil {
			t.Fatalf("request during failover: %v", err)
		}
		_ = resp.Body.Close()
		if got := resp.Header.Get("X-Backend"); got != "b" {
			t.Fatalf("expected only backend b during failover, got=%s", got)
		}
	}

	// Recover backend A and verify it re-enters rotation.
	aHealthy.Store(true)
	eventually(t, time.Second, 20*time.Millisecond, func() bool {
		return a.IsAlive()
	})

	recovered := false
	for i := 0; i < 8; i++ {
		resp, err := client.Get(proxy.URL + "/")
		if err != nil {
			t.Fatalf("request after recovery: %v", err)
		}
		_ = resp.Body.Close()
		if resp.Header.Get("X-Backend") == "a" {
			recovered = true
			break
		}
	}
	if !recovered {
		t.Fatal("backend a did not re-enter rotation after recovery")
	}
}

func TestIntegrationLeastConnectionsConcurrentLoad(t *testing.T) {
	backendA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		time.Sleep(15 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("from-a"))
	}))
	defer backendA.Close()

	backendB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		time.Sleep(15 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("from-b"))
	}))
	defer backendB.Close()

	cfg := &Config{
		Strategy: "least-connections",
		HealthCheck: HealthCheckConfig{
			Interval:      "1s",
			Timeout:       "100ms",
			FailThreshold: 1,
			PassThreshold: 1,
		},
		Backends: []BackendConfig{
			{Name: "a", URL: backendA.URL, Weight: 1, HealthPath: "/health"},
			{Name: "b", URL: backendB.URL, Weight: 1, HealthPath: "/health"},
		},
	}

	balancer, _, err := New(cfg, log.New(testWriter{t: t}, "", 0))
	if err != nil {
		t.Fatalf("new balancer: %v", err)
	}

	var aBackend *Backend
	var bBackend *Backend
	for _, be := range balancer.Backends() {
		switch be.Name {
		case "a":
			aBackend = be
		case "b":
			bBackend = be
		}
	}
	if aBackend == nil || bBackend == nil {
		t.Fatalf("failed to locate backend handles: a=%v b=%v", aBackend != nil, bBackend != nil)
	}

	// Simulate backend A already being saturated before the burst arrives.
	for i := 0; i < 100; i++ {
		aBackend.IncActive()
	}

	proxy := httptest.NewServer(balancer)
	defer proxy.Close()

	client := &http.Client{Timeout: 2 * time.Second}

	const totalRequests = 40
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(totalRequests)

	var routedToA atomic.Int64
	var routedToB atomic.Int64

	for i := 0; i < totalRequests; i++ {
		go func() {
			defer wg.Done()
			<-start
			resp, err := client.Get(proxy.URL + "/")
			if err != nil {
				t.Errorf("request failed: %v", err)
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("unexpected status: %d", resp.StatusCode)
			}
			switch resp.Header.Get("X-Backend") {
			case "a":
				routedToA.Add(1)
			case "b":
				routedToB.Add(1)
			default:
				t.Errorf("unexpected backend header: %q", resp.Header.Get("X-Backend"))
			}
		}()
	}

	close(start)
	wg.Wait()

	a := routedToA.Load()
	b := routedToB.Load()
	if a+b != totalRequests {
		t.Fatalf("unexpected routed accounting: a=%d b=%d total=%d", a, b, totalRequests)
	}
	if a != 0 {
		t.Fatalf("expected all burst traffic to avoid preloaded backend a, got a=%d b=%d", a, b)
	}

	// Remove synthetic load and ensure backend A can be selected again.
	for i := 0; i < 100; i++ {
		aBackend.DecActive()
	}

	recoveredA := false
	for i := 0; i < 8; i++ {
		resp, err := client.Get(proxy.URL + "/")
		if err != nil {
			t.Fatalf("request after releasing load: %v", err)
		}
		_ = resp.Body.Close()
		if resp.Header.Get("X-Backend") == "a" {
			recoveredA = true
			break
		}
	}
	if !recoveredA {
		t.Fatalf("backend a was not selected after synthetic load released; active a=%d b=%d", aBackend.ActiveConnections(), bBackend.ActiveConnections())
	}
}

func TestIntegrationLeastConnectionsWithInflightRequests(t *testing.T) {
	blockA := make(chan struct{})
	var aStarted atomic.Int64

	backendA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		aStarted.Add(1)
		<-blockA
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("from-a"))
	}))
	defer backendA.Close()

	backendB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("from-b"))
	}))
	defer backendB.Close()

	cfg := &Config{
		Strategy: "least-connections",
		HealthCheck: HealthCheckConfig{
			Interval:      "1s",
			Timeout:       "100ms",
			FailThreshold: 1,
			PassThreshold: 1,
		},
		Backends: []BackendConfig{
			{Name: "a", URL: backendA.URL, Weight: 1, HealthPath: "/health"},
			{Name: "b", URL: backendB.URL, Weight: 1, HealthPath: "/health"},
		},
	}

	balancer, _, err := New(cfg, log.New(testWriter{t: t}, "", 0))
	if err != nil {
		t.Fatalf("new balancer: %v", err)
	}

	proxy := httptest.NewServer(balancer)
	defer proxy.Close()

	client := &http.Client{Timeout: 2 * time.Second}

	// First request should route to backend A and stay in flight.
	started := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		resp, err := client.Get(proxy.URL + "/")
		if err != nil {
			errCh <- err
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errCh <- fmt.Errorf("unexpected status from in-flight request: %d", resp.StatusCode)
			return
		}
		errCh <- nil
	}()

	eventually(t, time.Second, 10*time.Millisecond, func() bool {
		if aStarted.Load() > 0 {
			close(started)
			return true
		}
		return false
	})
	<-started

	// While A is occupied, least-connections should prefer backend B.
	for i := 0; i < 12; i++ {
		resp, err := client.Get(proxy.URL + "/")
		if err != nil {
			t.Fatalf("request while A in-flight: %v", err)
		}
		_ = resp.Body.Close()
		if got := resp.Header.Get("X-Backend"); got != "b" {
			t.Fatalf("expected backend b while a has in-flight request, got=%s", got)
		}
	}

	close(blockA)
	if err := <-errCh; err != nil {
		t.Fatalf("in-flight request failed: %v", err)
	}

	// After A is released, it should appear in rotation again.
	foundA := false
	for i := 0; i < 8; i++ {
		resp, err := client.Get(proxy.URL + "/")
		if err != nil {
			t.Fatalf("post-release request failed: %v", err)
		}
		_ = resp.Body.Close()
		if resp.Header.Get("X-Backend") == "a" {
			foundA = true
			break
		}
	}
	if !foundA {
		t.Fatal("backend a did not re-enter selection after in-flight request completed")
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("lb: %s", p)
	return len(p), nil
}

func (w testWriter) String() string {
	return fmt.Sprintf("testWriter(%s)", w.t.Name())
}
