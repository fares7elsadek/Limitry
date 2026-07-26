package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/fares7elsadek/Limitry/internal/config"
	"github.com/redis/go-redis/v9"
)

type Limiter interface {
	Allow(ctx context.Context, clientID, route string) (allowed bool, retryAfter time.Duration, err error)
}

type Engine struct {
	redis    *redis.Client
	cfg      *config.Config
	failMode string
}

func NewEngine(cfg *config.Config) (*Engine, error) {
	client := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Addr,
	})


	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connecting to redis at %s: %w", cfg.Redis.Addr, err)
	}

	return &Engine{
		redis:    client,
		cfg:      cfg,
		failMode: cfg.Redis.FailMode,
	}, nil
}


func (e *Engine) Allow(ctx context.Context, clientID, route string) (bool, time.Duration, error) {
	routeCfg := e.cfg.FindRoute(route)
	if routeCfg == nil {
		// No limit configured for this route — allow by default
		return true, 0, nil
	}

	allowed, retryAfter, err := e.checkRedis(ctx, clientID, routeCfg)
	if err != nil {
		// Redis is down or errored — decide based on fail_mode
		if e.failMode == "open" {
			return true, 0, nil // let the request through
		}
		return false, 0, err // fail closed: reject when unsure
	}

	return allowed, retryAfter, nil
}


func (e *Engine) checkRedis(ctx context.Context, clientID string, r *config.RouteConfig) (bool, time.Duration, error) {
	key := fmt.Sprintf("ratelimit:%s:%s", r.Path, clientID)

	switch r.Algorithm {
		case "token_bucket":
			return tokenBucketAllow(ctx, e.redis, key, r.Limit, r.Window)
		case "sliding_window":
			return slidingWindowCounterAllow(ctx, e.redis, key, r.Limit, r.Window)
		default:
			return true, 0, nil
	}
}
