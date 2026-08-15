package proxy

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/fares7elsadek/Limitry/internal/limiter"
	"github.com/fares7elsadek/Limitry/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func NewHandler(backendURL string, lim limiter.Limiter, tracingEnabled bool, metrics *telemetry.Metrics) (http.Handler, error) {
	target, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}
	reverseProxy := httputil.NewSingleHostReverseProxy(target)

	if tracingEnabled {
		reverseProxy.Transport = otelhttp.NewTransport(http.DefaultTransport)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		clientID := r.Header.Get("X-Client-Id")
		if clientID == "" {
			clientID = r.RemoteAddr
		}

		ctx := r.Context()
		var span trace.Span

		if tracingEnabled {
			ctx, span = telemetry.Tracer().Start(ctx, "limitry.ratelimit.check",
				trace.WithAttributes(
					attribute.String("ratelimit.client_id", clientID),
					attribute.String("ratelimit.route", r.URL.Path),
				),
			)
		}

		limCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		allowed, retryAfter, limErr := lim.Allow(limCtx, clientID, r.URL.Path)

		if tracingEnabled {
			span.SetAttributes(attribute.Bool("ratelimit.allowed", allowed))
		}

		if limErr != nil {
			if tracingEnabled {
				span.RecordError(limErr)
				span.SetStatus(codes.Error, "rate limiter error")
				span.End()
			}
			http.Error(rec, "rate limiter error", http.StatusInternalServerError)
			logAndRecordMetrics(metrics, start, r, clientID, allowed, rec.status)
			return
		}

		if !allowed {
			if tracingEnabled {
				span.SetAttributes(
					attribute.Int64("ratelimit.retry_after_ms", retryAfter.Milliseconds()),
				)
				span.End()
			}
			rec.Header().Set("Retry-After", retryAfter.String())
			http.Error(rec, "rate limit exceeded", http.StatusTooManyRequests)
			logAndRecordMetrics(metrics, start, r, clientID, allowed, rec.status)
			return
		}

		if tracingEnabled {
			span.End()
		}

		reverseProxy.ServeHTTP(rec, r.WithContext(ctx))
		logAndRecordMetrics(metrics, start, r, clientID, allowed, rec.status)
	})

	if !tracingEnabled {
		return inner, nil
	}

	return otelhttp.NewHandler(inner, "limitry.proxy",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	), nil
}

func logAndRecordMetrics(metrics *telemetry.Metrics, start time.Time, r *http.Request, clientID string, allowed bool, status int) {
	duration := time.Since(start)

	if metrics != nil {
		metrics.RequestsTotal.WithLabelValues(r.URL.Path, strconv.FormatBool(allowed), "proxy").Inc()
		metrics.RequestDuration.WithLabelValues(r.URL.Path, strconv.Itoa(status), "proxy").Observe(duration.Seconds())
	}

	telemetry.Log().Info().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("client_id", clientID).
		Bool("allowed", allowed).
		Int("status", status).
		Dur("duration", duration).
		Msg("proxy request handled")
}