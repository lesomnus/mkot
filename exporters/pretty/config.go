package pretty

import (
	"context"
	"io"
	"os"

	"github.com/lesomnus/mkot"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Config struct {
	Output io.Writer
}

func (c *Config) Tracer(ctx context.Context) (trace.SpanExporter, func(ctx context.Context) error, error) {
	return nil, nil, nil
}

func (c *Config) Meter(ctx context.Context) (metric.Exporter, func(ctx context.Context) error, error) {
	return nil, nil, nil
}

func (c *Config) Reader(ctx context.Context) (metric.Reader, func(ctx context.Context) error, error) {
	return nil, nil, nil
}

func (c *Config) Logger(ctx context.Context) (log.Exporter, func(ctx context.Context) error, error) {
	return &LogExporter{Out: c.Output}, nil, nil
}

func init() {
	mkot.DefaultExporterRegistry.Set("pretty", mkot.ExporterConfigDecodeFunc(func() *Config {
		return &Config{Output: os.Stderr}
	}))
}
