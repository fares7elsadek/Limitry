package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/fares7elsadek/Limitry/internal/config"
	"github.com/fares7elsadek/Limitry/internal/telemetry"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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

	// --- redis eval span ---
	ctx, span := telemetry.Tracer().Start(ctx, "limitry.redis.eval",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("ratelimit.algorithm", r.Algorithm),
			attribute.String("ratelimit.key", key),
		),
	)
	defer span.End()

	var (
		allowed    bool
		retryAfter time.Duration
		err        error
	)

	switch r.Algorithm {
	case "token_bucket":
		allowed, retryAfter, err = tokenBucketAllow(ctx, e.redis, key, r.Limit, r.Window)
	case "sliding_window":
		allowed, retryAfter, err = slidingWindowCounterAllow(ctx, e.redis, key, r.Limit, r.Window)
	default:
		return true, 0, nil
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "redis eval failed")
		span.SetAttributes(attribute.Bool("ratelimit.failmode.activated", true))
		return false, 0, err
	}

	span.SetAttributes(attribute.Bool("ratelimit.allowed", allowed))
	return allowed, retryAfter, nil
}

