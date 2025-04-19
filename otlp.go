package mkot

import (
	"context"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

// OtlpExporterConfig defines common settings for a gRPC client configuration.
type OtlpExporterConfig struct {
	Enabled bool

	// The target to which the exporter is going to send traces or metrics,
	// using the gRPC protocol. The valid syntax is described at
	// https://github.com/grpc/grpc/blob/master/doc/naming.md.
	Endpoint string

	// The compression key for supported compression types within collector.
	Compression *string

	// Tls struct exposes TLS client configuration.
	Tls TlsConfig

	// The headers associated with gRPC requests.
	Headers map[string]string
}

func (c *OtlpExporterConfig) tracer(ctx context.Context) (trace.SpanExporter, error) {
	opts := c.traceOpts()
	return otlptracegrpc.NewUnstarted(opts...), nil
}

func (c *OtlpExporterConfig) traceOpts() []otlptracegrpc.Option {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(c.Endpoint),
	}
	opts = take(opts, c.Compression, otlptracegrpc.WithCompressor)
	if c.Tls.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else {
		// TODO:
		// opts = append(opts, otlptracegrpc.WithTLSCredentials())
	}

	return opts
}

func (c *OtlpExporterConfig) meter(ctx context.Context) (metric.Exporter, error) {
	opts := c.metricOpts()
	return otlpmetricgrpc.New(context.TODO(), opts...)
}

func (c *OtlpExporterConfig) metricOpts() []otlpmetricgrpc.Option {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(c.Endpoint),
	}
	if c.Compression != nil {
		opts = append(opts, otlpmetricgrpc.WithCompressor(*c.Compression))
	}
	if len(c.Headers) > 0 {
		opts = append(opts, otlpmetricgrpc.WithHeaders(c.Headers))
	}

	return opts
}

func (c *OtlpExporterConfig) logger(ctx context.Context) (log.Exporter, error) {
	opts := c.logOpts()
	return otlploggrpc.New(ctx, opts...)
}

func (c *OtlpExporterConfig) logOpts() []otlploggrpc.Option {
	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(c.Endpoint),
	}

	return opts
}
