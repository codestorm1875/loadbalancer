package lb

import (
	"testing"
)

func mustBackend(t *testing.T, name, rawURL string, weight int) *Backend {
	t.Helper()
	b, err := NewBackend(name, rawURL, weight, "/health")
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	return b
}

func TestRoundRobinStrategy(t *testing.T) {
	s := &RoundRobinStrategy{}
	b1 := mustBackend(t, "a", "http://127.0.0.1:9001", 1)
	b2 := mustBackend(t, "b", "http://127.0.0.1:9002", 1)
	backends := []*Backend{b1, b2}

	if got := s.Next(backends); got != b1 {
		t.Fatalf("first pick = %s, want a", got.Name)
	}
	if got := s.Next(backends); got != b2 {
		t.Fatalf("second pick = %s, want b", got.Name)
	}
}

func TestLeastConnectionsStrategy(t *testing.T) {
	s := &LeastConnectionsStrategy{}
	b1 := mustBackend(t, "a", "http://127.0.0.1:9001", 1)
	b2 := mustBackend(t, "b", "http://127.0.0.1:9002", 1)

	b1.IncActive()
	b1.IncActive()
	b2.IncActive()

	if got := s.Next([]*Backend{b1, b2}); got != b2 {
		t.Fatalf("pick = %s, want b", got.Name)
	}
}

func TestWeightedStrategyDistribution(t *testing.T) {
	s := NewWeightedRoundRobinStrategy()
	a := mustBackend(t, "a", "http://127.0.0.1:9001", 3)
	b := mustBackend(t, "b", "http://127.0.0.1:9002", 1)
	backends := []*Backend{a, b}

	counts := map[string]int{}
	for i := 0; i < 400; i++ {
		counts[s.Next(backends).Name]++
	}

	// Smooth weighted RR should be close to 3:1 over enough picks.
	if counts["a"] < 280 || counts["a"] > 320 {
		t.Fatalf("a count=%d out of expected range", counts["a"])
	}
	if counts["b"] < 80 || counts["b"] > 120 {
		t.Fatalf("b count=%d out of expected range", counts["b"])
	}
}
