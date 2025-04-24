package mkot

import (
	"context"

	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

type DebugExporterConfig struct {
	Enabled bool
}

func (c *DebugExporterConfig) Tracer(ctx context.Context) (trace.SpanExporter, func(ctx context.Context) error, error) {
	v, err := stdouttrace.New()
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func (c *DebugExporterConfig) Meter(ctx context.Context) (metric.Exporter, func(ctx context.Context) error, error) {
	v, err := stdoutmetric.New()
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func (c *DebugExporterConfig) Reader(ctx context.Context) (metric.Reader, func(ctx context.Context) error, error) {
	return nil, nil, nil
}

func (c *DebugExporterConfig) Logger(ctx context.Context) (log.Exporter, func(ctx context.Context) error, error) {
	v, err := stdoutlog.New()
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func init() {
	DefaultExporterRegistry.Set("debug", ExporterConfigDecodable[*DebugExporterConfig](func() *DebugExporterConfig {
		return &DebugExporterConfig{}
	}))
}
