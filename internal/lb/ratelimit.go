package lb

import (
	"net/http"
	"time"

	ratelimiter "github.com/codestorm1875/ratelimiter"
)

func BuildRateLimiterMiddleware(cfg RateLimitConfig, metrics *Metrics) (func(http.Handler) http.Handler, error) {
	halfLife, err := time.ParseDuration(cfg.HeatHalfLife)
	if err != nil {
		return nil, err
	}

	limiter, err := ratelimiter.New(ratelimiter.Config{
		Rate:         cfg.Rate,
		Burst:        cfg.Burst,
		HeatHalfLife: halfLife,
		HeatCost:     cfg.HeatCost,
		MaxKeys:      cfg.MaxKeys,
	})
	if err != nil {
		return nil, err
	}

	return func(next http.Handler) http.Handler {
		keyHeader := cfg.KeyHeader
		if keyHeader == "" {
			keyHeader = "X-Forwarded-For"
		}

		return limiter.Middleware(next,
			ratelimiter.WithKeyFunc(func(r *http.Request) string {
				if v := r.Header.Get(keyHeader); v != "" {
					return v
				}
				return ratelimiter.RemoteIPKey(r)
			}),
			ratelimiter.WithRejectFunc(func(w http.ResponseWriter, r *http.Request, d ratelimiter.Decision) {
				metrics.RateLimited.Add(1)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			}),
		)
	}, nil
}
