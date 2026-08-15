package telemetry

import (
	"context"
	"os"

	"github.com/fares7elsadek/Limitry/internal/config"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const Version = "1.0.0"

func NewResource(ctx context.Context, cfg config.TelemetryConfig) (*resource.Resource, error) {
	hostname, _ := os.Hostname()

	return resource.New(ctx,
		resource.WithFromEnv(),   // pull OTEL_RESOURCE_ATTRIBUTES from env
		resource.WithProcess(),   // pid, executable name
		resource.WithOS(),
		resource.WithContainer(), // container.id if in a container
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(Version),
			semconv.DeploymentEnvironment(cfg.Environment),
			semconv.HostName(hostname),
		),
	)
}