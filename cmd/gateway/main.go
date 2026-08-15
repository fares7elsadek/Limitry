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

	engine, err := limiter.NewEngine(cfg)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	log.Printf("connected to redis at %s", cfg.Redis.Addr)

	// telemetry
	ctx := context.Background()
	tracingEnabled := cfg.Telemetry.Enabled && cfg.Telemetry.Traces.Enabled

	if cfg.Telemetry.Enabled {

		res, err := telemetry.NewResource(ctx, cfg.Telemetry)
		if err != nil {
			log.Fatalf("resource: %v", err)
		}

		if cfg.Telemetry.Traces.Enabled {
			shutdownTrace, err := telemetry.InitTracer(ctx, res, cfg.Telemetry.Traces)
			if err != nil {
				log.Fatalf("tracer: %v", err)
			}
			defer shutdownTrace(ctx)
		}

	}

	// mode
	var handler http.Handler
	switch cfg.Mode {
	case "proxy":
		handler, err = proxy.NewHandler(cfg.Backend.URL, engine, tracingEnabled)
		if err != nil {
			log.Fatalf("failed to set up proxy: %v", err)
		}
		log.Printf("starting in PROXY mode → forwarding to %s", cfg.Backend.URL)
	case "check":
		handler = checkapi.NewHandler(engine, tracingEnabled)
		log.Println("starting in CHECK mode → POST /check")
	default:
		log.Fatalf("unknown mode: %s", cfg.Mode)
	}

	log.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}

}

