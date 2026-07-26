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

type MetricsConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
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