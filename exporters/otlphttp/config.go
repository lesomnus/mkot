package otlphttp

import (
	"context"
	"net/http"

	"github.com/lesomnus/mkot"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Config struct {
	HttpClient *http.Client

	Endpoint string

	// The URL to send traces to. If omitted the Endpoint + "/v1/traces" will be used.
	TracesEndpoint string `yaml:"traces_endpoint"`

	// The URL to send metrics to. If omitted the Endpoint + "/v1/metrics" will be used.
	MetricsEndpoint string `yaml:"metrics_endpoint"`

	// The URL to send logs to. If omitted the Endpoint + "/v1/logs" will be used.
	LogsEndpoint string `yaml:"logs_endpoint"`

	Tls mkot.TlsConfig
}

func (c *Config) Tracer(ctx context.Context) (trace.SpanExporter, func(ctx context.Context) error, error) {
	opts := c.traceOpts()
	v := otlptracehttp.NewUnstarted(opts...)
	return v, v.Start, nil
}

func (c *Config) traceOpts() []otlptracehttp.Option {
	opts := []otlptracehttp.Option{}
	if c.HttpClient != nil {
		opts = append(opts, otlptracehttp.WithHTTPClient(c.HttpClient))
	}

	ep := c.TracesEndpoint
	if ep == "" {
		ep = c.Endpoint + "/v1/traces"
	}
	opts = append(opts, otlptracehttp.WithEndpointURL(ep))

	if c.Tls.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	return opts
}

func (c *Config) Meter(ctx context.Context) (metric.Exporter, func(ctx context.Context) error, error) {
	opts := c.metricOpts()
	v, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func (c *Config) Reader(ctx context.Context) (metric.Reader, func(ctx context.Context) error, error) {
	return nil, nil, nil
}

func (c *Config) metricOpts() []otlpmetrichttp.Option {
	opts := []otlpmetrichttp.Option{}
	if c.HttpClient != nil {
		opts = append(opts, otlpmetrichttp.WithHTTPClient(c.HttpClient))
	}

	ep := c.TracesEndpoint
	if ep == "" {
		ep = c.Endpoint + "/v1/metrics"
	}
	opts = append(opts, otlpmetrichttp.WithEndpointURL(ep))

	if c.Tls.Insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	return opts
}

func (c *Config) Logger(ctx context.Context) (log.Exporter, func(ctx context.Context) error, error) {
	opts := c.logOpts()
	v, err := otlploghttp.New(ctx, opts...)
	if err != nil {
		return nil, nil, err
	}
	return v, nil, nil
}

func (c *Config) logOpts() []otlploghttp.Option {
	opts := []otlploghttp.Option{}
	if c.HttpClient != nil {
		opts = append(opts, otlploghttp.WithHTTPClient(c.HttpClient))
	}

	ep := c.TracesEndpoint
	if ep == "" {
		ep = c.Endpoint + "/v1/logs"
	}
	opts = append(opts, otlploghttp.WithEndpointURL(ep))

	if c.Tls.Insecure {
		opts = append(opts, otlploghttp.WithInsecure())
	}

	return opts
}

func init() {
	mkot.DefaultExporterRegistry.Set("otlphttp", mkot.ExporterConfigDecodeFunc(func() *Config {
		return &Config{}
	}))
}
