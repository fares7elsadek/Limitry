package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/fares7elsadek/Limitry/internal/checkapi"
	"github.com/fares7elsadek/Limitry/internal/config"
	"github.com/fares7elsadek/Limitry/internal/limiter"
	"github.com/fares7elsadek/Limitry/internal/proxy"
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

	var handler http.Handler
	switch cfg.Mode {
		case "proxy":
			handler, err = proxy.NewHandler(cfg.Backend.URL, engine)
			if err != nil {
				log.Fatalf("failed to set up proxy: %v", err)
			}
			log.Printf("starting in PROXY mode → forwarding to %s", cfg.Backend.URL)
		case "check":
			handler = checkapi.NewHandler(engine)
			log.Println("starting in CHECK mode → POST /check")
		default:
			log.Fatalf("unknown mode: %s", cfg.Mode)
	}

	log.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}

}

