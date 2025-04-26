package debug

import (
	"context"

	"github.com/lesomnus/mkot"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Config struct {
	Enabled bool
}

func (c *Config) Tracer(ctx context.Context) (trace.SpanExporter, func(ctx context.Context) error, error) {
	v, err := stdouttrace.New()
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func (c *Config) Meter(ctx context.Context) (metric.Exporter, func(ctx context.Context) error, error) {
	v, err := stdoutmetric.New()
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func (c *Config) Reader(ctx context.Context) (metric.Reader, func(ctx context.Context) error, error) {
	return nil, nil, nil
}

func (c *Config) Logger(ctx context.Context) (log.Exporter, func(ctx context.Context) error, error) {
	v, err := stdoutlog.New()
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func init() {
	mkot.DefaultExporterRegistry.Set("debug", mkot.ExporterConfigDecodeFunc(func() *Config {
		return &Config{}
	}))
}
