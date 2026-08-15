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
	metrics  *telemetry.Metrics
}

func NewEngine(cfg *config.Config, metrics *telemetry.Metrics) (*Engine, error) {
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
		metrics:  metrics,
	}, nil
}

func (e *Engine) Allow(ctx context.Context, clientID, route string) (bool, time.Duration, error) {
	routeCfg := e.cfg.FindRoute(route)
	if routeCfg == nil {
		return true, 0, nil
	}

	start := time.Now()
	allowed, retryAfter, err := e.checkRedis(ctx, clientID, routeCfg)
	duration := time.Since(start)

	if e.metrics != nil {
		e.metrics.RatelimitDuration.WithLabelValues(route, routeCfg.Algorithm).Observe(duration.Seconds())
	}

	if err != nil {
		if e.metrics != nil {
			e.metrics.RedisErrorsTotal.WithLabelValues(route, e.failMode).Inc()
		}
		telemetry.Log().Error().
			Err(err).
			Str("route", route).
			Str("client_id", clientID).
			Str("failmode", e.failMode).
			Msg("redis error during rate-limit check")

		if e.failMode == "open" {
			return true, 0, nil
		}
		return false, 0, err
	}

	return allowed, retryAfter, nil
}

func (e *Engine) checkRedis(ctx context.Context, clientID string, r *config.RouteConfig) (bool, time.Duration, error) {
	key := fmt.Sprintf("ratelimit:%s:%s", r.Path, clientID)

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

	telemetry.Log().Debug().
		Str("key", key).
		Str("algorithm", r.Algorithm).
		Bool("allowed", allowed).
		Str("retry_after", retryAfter.String()).
		Msg("rate-limit evaluated")

	return allowed, retryAfter, nil
}
