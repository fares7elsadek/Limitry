package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)


type Config struct {
	Mode    string        `yaml:"mode"`
	Backend BackendConfig `yaml:"backend"`
	Redis   RedisConfig   `yaml:"redis"`
	Routes  []RouteConfig `yaml:"routes"`
	Metrics MetricsConfig `yaml:"metrics"`
	Telemetry  TelemetryConfig  `yaml:"telemetry"`
}

type BackendConfig struct {
	URL string `yaml:"url"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	FailMode string `yaml:"fail_mode"` // "open" or "closed"
}

type RouteConfig struct {
	Path      string        `yaml:"path"`
	Algorithm string        `yaml:"algorithm"` // "token_bucket" or "sliding_window"
	Limit     int           `yaml:"limit"`
	Window    time.Duration `yaml:"window"` // yaml.v3 parses "60s" into time.Duration automatically
}


type TelemetryConfig struct {
	Enabled     bool          `yaml:"enabled"`
	ServiceName string        `yaml:"service_name"`
	Environment string        `yaml:"environment"`
	Traces      TraceConfig   `yaml:"traces"`
	Metrics     MetricConfig  `yaml:"metrics"`
	Logs        LogConfig     `yaml:"logs"`
}

type MetricsConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

type TraceConfig struct {
	Enabled       bool    `yaml:"enabled"`
	Endpoint      string  `yaml:"endpoint"`
	Insecure      bool    `yaml:"insecure"`
	SamplingRatio float64 `yaml:"sampling_ratio"`
}

type MetricConfig struct {
	Enabled bool `yaml:"enabled"`
}

type LogConfig struct {
	Enabled bool   `yaml:"enabled"`
	Level   string `yaml:"level"`
}


func Load(path string) (*Config, error){
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}


func (c *Config) validate() error {
	if c.Mode != "proxy" && c.Mode != "check" {
		return fmt.Errorf("mode must be 'proxy' or 'check', got %q", c.Mode)
	}
	if c.Mode == "proxy" && c.Backend.URL == "" {
		return fmt.Errorf("backend.url is required in proxy mode")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr is required")
	}
	if c.Redis.FailMode != "open" && c.Redis.FailMode != "closed" {
		return fmt.Errorf("redis.fail_mode must be 'open' or 'closed'")
	}
	for _, r := range c.Routes {
		if r.Algorithm != "token_bucket" && r.Algorithm != "sliding_window" {
			return fmt.Errorf("route %s: unknown algorithm %q", r.Path, r.Algorithm)
		}
		if r.Limit <= 0 {
			return fmt.Errorf("route %s: limit must be > 0", r.Path)
		}
	}
	if err := c.Telemetry.validate(); err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	return nil
}

func (t *TelemetryConfig) validate() error {
	if !t.Enabled {
		return nil
	}
	if t.ServiceName == "" {
		return fmt.Errorf("service_name is required when telemetry is enabled")
	}
	if t.Traces.Enabled {
		if t.Traces.Endpoint == "" {
			return fmt.Errorf("traces.endpoint is required when traces are enabled")
		}
		if t.Traces.SamplingRatio < 0 || t.Traces.SamplingRatio > 1 {
			return fmt.Errorf("traces.sampling_ratio must be between 0 and 1, got %v", t.Traces.SamplingRatio)
		}
	}
	if t.Logs.Enabled {
		switch t.Logs.Level {
		case "debug", "info", "warn", "error":
			// ok
		default:
			return fmt.Errorf("logs.level must be one of debug/info/warn/error, got %q", t.Logs.Level)
		}
	}
	return nil
}

func (c *Config) FindRoute(path string) *RouteConfig {
	for i := range c.Routes {
		if c.Routes[i].Path == path {
			return &c.Routes[i]
		}
	}
	return nil
}