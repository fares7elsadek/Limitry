package telemetry

import (
	"io"
	"os"
	"time"

	"github.com/fares7elsadek/Limitry/internal/config"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
)

var logger zerolog.Logger

func init() {
	logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}

func InitLogger(cfg config.LogConfig) {
	if !cfg.Enabled {
		logger = zerolog.New(io.Discard)
		return
	}

	switch cfg.Level {
		case "debug":
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		case "info":
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		case "warn":
			zerolog.SetGlobalLevel(zerolog.WarnLevel)
		case "error":
			zerolog.SetGlobalLevel(zerolog.ErrorLevel)
		default:
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	var output io.Writer = os.Stdout

	format := cfg.Format
	if format == "" {
		format = "json"
	}

	if format == "console" || (format == "" && isatty.IsTerminal(os.Stdout.Fd())) {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	logger = zerolog.New(output).With().Timestamp().Logger()
}

func Log() *zerolog.Logger {
	return &logger
}
