package lb

import (
	"sync"
	"sync/atomic"
)

type Strategy interface {
	Name() string
	Next(backends []*Backend) *Backend
}

type RoundRobinStrategy struct {
	counter atomic.Uint64
}

func (s *RoundRobinStrategy) Name() string { return "round-robin" }

func (s *RoundRobinStrategy) Next(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}
	n := s.counter.Add(1)
	idx := int((n - 1) % uint64(len(backends)))
	return backends[idx]
}

type LeastConnectionsStrategy struct{}

func (s *LeastConnectionsStrategy) Name() string { return "least-connections" }

func (s *LeastConnectionsStrategy) Next(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}
	selected := backends[0]
	minActive := selected.ActiveConnections()
	for _, b := range backends[1:] {
		if active := b.ActiveConnections(); active < minActive {
			selected = b
			minActive = active
		}
	}
	return selected
}

// WeightedRoundRobinStrategy uses smooth weighted round robin.
type WeightedRoundRobinStrategy struct {
	mu      sync.Mutex
	current map[*Backend]int
}

func NewWeightedRoundRobinStrategy() *WeightedRoundRobinStrategy {
	return &WeightedRoundRobinStrategy{current: make(map[*Backend]int)}
}

func (s *WeightedRoundRobinStrategy) Name() string { return "weighted" }

func (s *WeightedRoundRobinStrategy) Next(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	totalWeight := 0
	var best *Backend
	bestWeight := 0

	for _, b := range backends {
		totalWeight += b.Weight
		s.current[b] += b.Weight
		if best == nil || s.current[b] > bestWeight {
			best = b
			bestWeight = s.current[b]
		}
	}
	if best == nil {
		return nil
	}
	s.current[best] -= totalWeight
	return best
}

func NewStrategy(name string) (Strategy, error) {
	switch name {
	case "round-robin":
		return &RoundRobinStrategy{}, nil
	case "least-connections":
		return &LeastConnectionsStrategy{}, nil
	case "weighted":
		return NewWeightedRoundRobinStrategy(), nil
	default:
		return nil, ErrUnknownStrategy{name: name}
	}
}

type ErrUnknownStrategy struct {
	name string
}

func (e ErrUnknownStrategy) Error() string {
	return "unknown strategy: " + e.name
}
