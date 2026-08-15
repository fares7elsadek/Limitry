package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/fares7elsadek/Limitry/internal/checkapi"
	"github.com/fares7elsadek/Limitry/internal/config"
	"github.com/fares7elsadek/Limitry/internal/limiter"
	"github.com/fares7elsadek/Limitry/internal/proxy"
	"github.com/fares7elsadek/Limitry/internal/telemetry"
)

func main() {

	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	telemetry.InitLogger(cfg.Telemetry.Logs)
	logger := telemetry.Log()

	var metrics *telemetry.Metrics
	if cfg.Telemetry.Enabled && cfg.Telemetry.Metrics.Enabled {
		metrics, err = telemetry.InitMetrics(cfg.Telemetry.Metrics)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to initialize metrics")
		}
		logger.Info().Int("port", cfg.Telemetry.Metrics.Port).Msg("metrics server started")
	}

	engine, err := limiter.NewEngine(cfg, metrics)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to redis")
	}
	logger.Info().Str("addr", cfg.Redis.Addr).Msg("connected to redis")

	// telemetry
	ctx := context.Background()
	tracingEnabled := cfg.Telemetry.Enabled && cfg.Telemetry.Traces.Enabled

	if cfg.Telemetry.Enabled {

		res, err := telemetry.NewResource(ctx, cfg.Telemetry)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to create resource")
		}

		if cfg.Telemetry.Traces.Enabled {
			shutdownTrace, err := telemetry.InitTracer(ctx, res, cfg.Telemetry.Traces)
			if err != nil {
				logger.Fatal().Err(err).Msg("failed to initialize tracer")
			}
			defer shutdownTrace(ctx)
		}

	}

	// mode
	var handler http.Handler
	switch cfg.Mode {
		case "proxy":
			handler, err = proxy.NewHandler(cfg.Backend.URL, engine, tracingEnabled, metrics)
			if err != nil {
				logger.Fatal().Err(err).Msg("failed to set up proxy")
			}
			logger.Info().Str("forward_to", cfg.Backend.URL).Msg("starting in PROXY mode")
		case "check":
			handler = checkapi.NewHandler(engine, tracingEnabled, metrics)
			logger.Info().Msg("starting in CHECK mode → POST /check")
		default:
			logger.Fatal().Str("mode", cfg.Mode).Msg("unknown mode")
	}

	logger.Info().Msg("listening on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		logger.Fatal().Err(err).Msg("server error")
	}

}

