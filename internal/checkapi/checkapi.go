package checkapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/fares7elsadek/Limitry/internal/limiter"
	"github.com/fares7elsadek/Limitry/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type checkRequest struct {
	ClientID string `json:"client_id"`
	Route    string `json:"route"`
}

type checkResponse struct {
	Allowed           bool `json:"allowed"`
	RetryAfterSeconds int  `json:"retry_after_seconds"`
}

func NewHandler(lim limiter.Limiter, tracingEnabled bool) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if tracingEnabled {
			parentSpan := trace.SpanFromContext(r.Context())
			var req checkRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				parentSpan.RecordError(err)
				parentSpan.SetStatus(codes.Error, "invalid request body")
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}

			ctx, span := telemetry.Tracer().Start(r.Context(), "limitry.ratelimit.check",
				trace.WithAttributes(
					attribute.String("ratelimit.client_id", req.ClientID),
					attribute.String("ratelimit.route", req.Route),
				),
			)

			limCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			allowed, retryAfter, limErr := lim.Allow(limCtx, req.ClientID, req.Route)

			span.SetAttributes(attribute.Bool("ratelimit.allowed", allowed))

			if limErr != nil {
				span.RecordError(limErr)
				span.SetStatus(codes.Error, "rate limiter error")
				span.End()
				http.Error(w, "rate limiter error", http.StatusInternalServerError)
				return
			}

			if !allowed {
				span.SetAttributes(
					attribute.Int64("ratelimit.retry_after_ms", retryAfter.Milliseconds()),
				)
			}
			span.End()

			w.Header().Set("Content-Type", "application/json")
			if !allowed {
				w.WriteHeader(http.StatusTooManyRequests)
			}
			json.NewEncoder(w).Encode(checkResponse{
				Allowed:           allowed,
				RetryAfterSeconds: int(retryAfter.Seconds()),
			})
			return
		}

		var req checkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		allowed, retryAfter, err := lim.Allow(ctx, req.ClientID, req.Route)
		if err != nil {
			http.Error(w, "rate limiter error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if !allowed {
			w.WriteHeader(http.StatusTooManyRequests)
		}
		json.NewEncoder(w).Encode(checkResponse{
			Allowed:           allowed,
			RetryAfterSeconds: int(retryAfter.Seconds()),
		})
	})

	if !tracingEnabled {
		return inner
	}

	return otelhttp.NewHandler(inner, "limitry.check",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)
}