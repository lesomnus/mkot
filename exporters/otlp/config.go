package otlp

import (
	"context"

	"github.com/lesomnus/mkot"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

// Config defines common settings for a gRPC client configuration.
type Config struct {
	Enabled bool

	// The target to which the exporter is going to send traces or metrics,
	// using the gRPC protocol. The valid syntax is described at
	// https://github.com/grpc/grpc/blob/master/doc/naming.md.
	Endpoint string

	// The compression key for supported compression types within collector.
	Compression *string

	// Tls struct exposes TLS client configuration.
	Tls mkot.TlsConfig

	// The headers associated with gRPC requests.
	Headers map[string]string
}

func (c *Config) Tracer(ctx context.Context) (trace.SpanExporter, func(ctx context.Context) error, error) {
	opts := c.traceOpts()
	v := otlptracegrpc.NewUnstarted(opts...)
	return v, v.Start, nil
}

func (c *Config) traceOpts() []otlptracegrpc.Option {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(c.Endpoint),
	}
	if c.Compression != nil {
		opts = append(opts, otlptracegrpc.WithCompressor(*c.Compression))
	}
	if c.Tls.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else {
		// TODO:
		// opts = append(opts, otlptracegrpc.WithTLSCredentials())
	}

	return opts
}

func (c *Config) Meter(ctx context.Context) (metric.Exporter, func(ctx context.Context) error, error) {
	opts := c.metricOpts()
	v, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func (c *Config) Reader(ctx context.Context) (metric.Reader, func(ctx context.Context) error, error) {
	return nil, nil, nil
}

func (c *Config) metricOpts() []otlpmetricgrpc.Option {
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

func (c *Config) Logger(ctx context.Context) (log.Exporter, func(ctx context.Context) error, error) {
	opts := c.logOpts()
	v, err := otlploggrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func (c *Config) logOpts() []otlploggrpc.Option {
	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(c.Endpoint),
	}

	return opts
}

func init() {
	mkot.DefaultExporterRegistry.Set("otlp", mkot.ExporterConfigDecodeFunc(func() *Config {
		return &Config{}
	}))
}
