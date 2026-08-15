package proxy

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/fares7elsadek/Limitry/internal/limiter"
	"github.com/fares7elsadek/Limitry/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func NewHandler(backendURL string, lim limiter.Limiter, tracingEnabled bool) (http.Handler, error) {
	target, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}
	reverseProxy := httputil.NewSingleHostReverseProxy(target)

	if tracingEnabled {
		reverseProxy.Transport = otelhttp.NewTransport(http.DefaultTransport)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			http.Error(w, "rate limiter error", http.StatusInternalServerError)
			return
		}

		if !allowed {
			if tracingEnabled {
				span.SetAttributes(
					attribute.Int64("ratelimit.retry_after_ms", retryAfter.Milliseconds()),
				)
				span.End()
			}
			w.Header().Set("Retry-After", retryAfter.String())
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		if tracingEnabled {
			span.End()
		}

		reverseProxy.ServeHTTP(w, r.WithContext(ctx))
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