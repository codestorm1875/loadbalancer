package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/codestorm1875/loadbalancer/internal/lb"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to YAML/JSON config")
	flag.Parse()

	logger := log.New(os.Stdout, "loadbalancer ", log.LstdFlags|log.Lmicroseconds)

	cfg, err := lb.LoadConfig(*configPath)
	if err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	balancer, checker, err := lb.New(cfg, logger)
	if err != nil {
		logger.Fatalf("failed to initialize balancer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	balancer.StartHealthChecks(ctx, checker)

	rootHandler := http.Handler(balancer)
	if cfg.RateLimit.Enabled {
		mw, err := lb.BuildRateLimiterMiddleware(cfg.RateLimit, balancer.Metrics())
		if err != nil {
			logger.Fatalf("failed to initialize rate limiter: %v", err)
		}
		rootHandler = mw(rootHandler)
	}

	mux := http.NewServeMux()
	mux.Handle("/", rootHandler)
	mux.Handle("/metrics", balancer.Metrics().Handler(balancer.Backends()))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}

	go func() {
		logger.Printf("listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Printf("shutdown signal received")

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("graceful shutdown failed: %v", err)
	}
}
