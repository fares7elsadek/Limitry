package proxy

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/fares7elsadek/Limitry/internal/limiter"
)

func NewHandler(backendURL string, lim limiter.Limiter) (http.Handler, error) {
	target, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}
	reverseProxy := httputil.NewSingleHostReverseProxy(target)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID := r.Header.Get("X-Client-Id")
		if clientID == "" {
			clientID = r.RemoteAddr // fallback if no client id header sent
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		allowed, retryAfter, err := lim.Allow(ctx, clientID, r.URL.Path)
		if err != nil {
			http.Error(w, "rate limiter error", http.StatusInternalServerError)
			return
		}

		if !allowed {
			w.Header().Set("Retry-After", retryAfter.String())
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		reverseProxy.ServeHTTP(w, r) // forwards + streams response back
	}), nil
}