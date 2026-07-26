package checkapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/fares7elsadek/Limitry/internal/limiter"
)

type checkRequest struct {
	ClientID string `json:"client_id"`
	Route    string `json:"route"`
}

type checkResponse struct {
	Allowed           bool `json:"allowed"`
	RetryAfterSeconds int  `json:"retry_after_seconds"`
}

func NewHandler(lim limiter.Limiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
}