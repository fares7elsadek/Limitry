package telemetry

import (
	"fmt"
	"net/http"

	"github.com/fares7elsadek/Limitry/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	RequestsTotal     *prometheus.CounterVec
	RatelimitDuration *prometheus.HistogramVec
	RedisErrorsTotal  *prometheus.CounterVec
	RequestDuration   *prometheus.HistogramVec
}

func InitMetrics(cfg config.MetricConfig) (*Metrics, error) {
	m := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "limitry_requests_total",
				Help: "Total rate-limit decisions",
			},
			[]string{"route", "allowed", "mode"},
		),
		RatelimitDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "limitry_ratelimit_duration_seconds",
				Help:    "Time spent evaluating the rate-limit decision (Redis round-trip)",
				Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
			},
			[]string{"route", "algorithm"},
		),
		RedisErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "limitry_redis_errors_total",
				Help: "Total Redis errors during rate-limit evaluation",
			},
			[]string{"route", "failmode"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "limitry_request_duration_seconds",
				Help:    "Total HTTP request duration",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"route", "status_code", "mode"},
		),
	}

	prometheus.MustRegister(m.RequestsTotal)
	prometheus.MustRegister(m.RatelimitDuration)
	prometheus.MustRegister(m.RedisErrorsTotal)
	prometheus.MustRegister(m.RequestDuration)

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		addr := fmt.Sprintf(":%d", cfg.Port)
		http.ListenAndServe(addr, mux)
	}()

	return m, nil
}
