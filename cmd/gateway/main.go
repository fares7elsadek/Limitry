package main

import (
	"flag"
	"log"

	"github.com/fares7elsadek/Limitry/internal/config"
)

func main() {

	configPath := flag.String("config","config.yaml","path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Printf("loaded config: mode=%s redis=%s routes=%d",
		cfg.Mode, cfg.Redis.Addr, len(cfg.Routes))

	switch cfg.Mode {
		case "proxy":
			log.Printf("starting in PROXY mode, forwarding to %s", cfg.Backend.URL)
			runProxyMode(cfg)
		case "check":
			log.Println("starting in CHECK mode, decision-only endpoint")
			runCheckMode(cfg)
	}

}

// placeholder — we'll build this out with httputil.ReverseProxy next
func runProxyMode(cfg *config.Config) {
	// TODO: set up reverse proxy handler + rate limit middleware
}

// placeholder — we'll build this out with the /check HTTP handler next
func runCheckMode(cfg *config.Config) {
	// TODO: set up /check endpoint
}