package lb

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config describes all runtime settings for the load balancer.
type Config struct {
	ListenAddr  string            `json:"listen_addr" yaml:"listen_addr"`
	Strategy    string            `json:"strategy" yaml:"strategy"`
	Backends    []BackendConfig   `json:"backends" yaml:"backends"`
	HealthCheck HealthCheckConfig `json:"health_check" yaml:"health_check"`
	RateLimit   RateLimitConfig   `json:"rate_limit" yaml:"rate_limit"`
}

type BackendConfig struct {
	Name       string `json:"name" yaml:"name"`
	URL        string `json:"url" yaml:"url"`
	Weight     int    `json:"weight" yaml:"weight"`
	HealthPath string `json:"health_path" yaml:"health_path"`
}

type HealthCheckConfig struct {
	Interval      string `json:"interval" yaml:"interval"`
	Timeout       string `json:"timeout" yaml:"timeout"`
	FailThreshold int    `json:"fail_threshold" yaml:"fail_threshold"`
	PassThreshold int    `json:"pass_threshold" yaml:"pass_threshold"`
}

type RateLimitConfig struct {
	Enabled      bool    `json:"enabled" yaml:"enabled"`
	Rate         float64 `json:"rate" yaml:"rate"`
	Burst        int     `json:"burst" yaml:"burst"`
	HeatHalfLife string  `json:"heat_half_life" yaml:"heat_half_life"`
	HeatCost     float64 `json:"heat_cost" yaml:"heat_cost"`
	MaxKeys      int     `json:"max_keys" yaml:"max_keys"`
	KeyHeader    string  `json:"key_header" yaml:"key_header"`
}

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.Strategy == "" {
		cfg.Strategy = "round-robin"
	}
	if cfg.HealthCheck.Interval == "" {
		cfg.HealthCheck.Interval = "5s"
	}
	if cfg.HealthCheck.Timeout == "" {
		cfg.HealthCheck.Timeout = "2s"
	}
	if cfg.HealthCheck.FailThreshold <= 0 {
		cfg.HealthCheck.FailThreshold = 2
	}
	if cfg.HealthCheck.PassThreshold <= 0 {
		cfg.HealthCheck.PassThreshold = 1
	}
	if cfg.RateLimit.Enabled {
		if cfg.RateLimit.Rate <= 0 {
			cfg.RateLimit.Rate = 100
		}
		if cfg.RateLimit.Burst <= 0 {
			cfg.RateLimit.Burst = 50
		}
		if cfg.RateLimit.HeatHalfLife == "" {
			cfg.RateLimit.HeatHalfLife = "5s"
		}
		if cfg.RateLimit.HeatCost <= 0 {
			cfg.RateLimit.HeatCost = 1.25
		}
		if cfg.RateLimit.MaxKeys <= 0 {
			cfg.RateLimit.MaxKeys = 4096
		}
	}
}

func validateConfig(cfg *Config) error {
	if len(cfg.Backends) == 0 {
		return errors.New("config requires at least one backend")
	}
	validStrategies := map[string]bool{
		"round-robin":       true,
		"least-connections": true,
		"weighted":          true,
	}
	if !validStrategies[cfg.Strategy] {
		return fmt.Errorf("unsupported strategy %q", cfg.Strategy)
	}

	for i, b := range cfg.Backends {
		if strings.TrimSpace(b.URL) == "" {
			return fmt.Errorf("backend[%d] url is required", i)
		}
		if _, err := url.ParseRequestURI(b.URL); err != nil {
			return fmt.Errorf("backend[%d] url is invalid: %w", i, err)
		}
		if b.Weight <= 0 {
			cfg.Backends[i].Weight = 1
		}
	}

	if _, err := time.ParseDuration(cfg.HealthCheck.Interval); err != nil {
		return fmt.Errorf("health_check.interval is invalid: %w", err)
	}
	if _, err := time.ParseDuration(cfg.HealthCheck.Timeout); err != nil {
		return fmt.Errorf("health_check.timeout is invalid: %w", err)
	}

	if cfg.RateLimit.Enabled {
		if _, err := time.ParseDuration(cfg.RateLimit.HeatHalfLife); err != nil {
			return fmt.Errorf("rate_limit.heat_half_life is invalid: %w", err)
		}
	}

	return nil
}
