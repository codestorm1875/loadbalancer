package lb

import (
	"net/url"
	"sync/atomic"
)

// Backend represents one upstream service target.
type Backend struct {
	Name       string
	URL        *url.URL
	Weight     int
	HealthPath string

	alive  atomic.Bool
	active atomic.Int64
}

func NewBackend(name string, rawURL string, weight int, healthPath string) (*Backend, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if weight <= 0 {
		weight = 1
	}
	b := &Backend{
		Name:       name,
		URL:        u,
		Weight:     weight,
		HealthPath: healthPath,
	}
	b.alive.Store(true)
	return b, nil
}

func (b *Backend) IsAlive() bool {
	return b.alive.Load()
}

func (b *Backend) SetAlive(v bool) {
	b.alive.Store(v)
}

func (b *Backend) ActiveConnections() int64 {
	return b.active.Load()
}

func (b *Backend) IncActive() {
	b.active.Add(1)
}

func (b *Backend) DecActive() {
	b.active.Add(-1)
}
