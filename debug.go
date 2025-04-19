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

func (c *DebugExporterConfig) tracer(ctx context.Context) (trace.SpanExporter, error) {
	return stdouttrace.New()
}

func (c *DebugExporterConfig) meter(ctx context.Context) (metric.Exporter, error) {
	return stdoutmetric.New()
}

func (c *DebugExporterConfig) logger(ctx context.Context) (log.Exporter, error) {
	return stdoutlog.New()
}
