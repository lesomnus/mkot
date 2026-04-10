package otlp

import (
	"context"
	"fmt"
	"time"

	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/opaque"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var _ mkot.ExporterConfig = (*ExporterConfig)(nil)

// ExporterConfig defines common settings for a gRPC client configuration.
type ExporterConfig struct {
	// Copied from https://github.com/open-telemetry/opentelemetry-collector/blob/41c3a7661559975374656a2fe886c6de0b726052/config/confighttp/client.go

	// The target to which the exporter is going to send traces or metrics,
	// using the gRPC protocol. The valid syntax is described at
	// https://github.com/grpc/grpc/blob/master/doc/naming.md.
	Endpoint string `yaml:"endpoint,omitempty"`

	// The compression key for supported compression types within collector.
	Compression string `yaml:"compression,omitempty"`

	// TLS struct exposes TLS client configuration.
	TLS *mkot.ClientTlsConfig `yaml:"tls,omitempty"`

	// The keepalive parameters for gRPC client. See grpc.WithKeepaliveParams.
	// (https://godoc.org/google.golang.org/grpc#WithKeepaliveParams).
	Keepalive *KeepaliveConfig `yaml:"keepalive,omitempty"`

	// ReadBufferSize for gRPC client. See grpc.WithReadBufferSize.
	// (https://godoc.org/google.golang.org/grpc#WithReadBufferSize).
	ReadBufferSize int `yaml:"read_buffer_size,omitempty"`

	// WriteBufferSize for gRPC gRPC. See grpc.WithWriteBufferSize.
	// (https://godoc.org/google.golang.org/grpc#WithWriteBufferSize).
	WriteBufferSize int `yaml:"write_buffer_size,omitempty"`

	// WaitForReady parameter configures client to wait for ready state before sending data.
	// (https://github.com/grpc/grpc/blob/master/doc/wait-for-ready.md)
	WaitForReady bool `yaml:"wait_for_ready,omitempty"`

	// The headers associated with gRPC requests.
	Headers opaque.MapList `yaml:"headers,omitempty"`

	// Sets the balancer in grpclb_policy to discover the servers. Default is pick_first.
	// https://github.com/grpc/grpc-go/blob/master/examples/features/load_balancing/README.md
	BalancerName string `yaml:"balancer_name,omitempty"`

	// WithAuthority parameter configures client to rewrite ":authority" header
	// (godoc.org/google.golang.org/grpc#WithAuthority)
	Authority string `yaml:"authority,omitempty"`

	// // Auth configuration for outgoing RPCs.
	// Auth configoptional.Optional[configauth.Config] `yaml:"auth,omitempty"`

	// // Middlewares for the gRPC client.
	// Middlewares []configmiddleware.Config `yaml:"middlewares,omitempty"`

	Retry mkot.RetryConfig `yaml:"retry_on_failure,omitempty"`
	Queue mkot.QueueConfig `yaml:"sending_queue,omitempty"`
}

func (e ExporterConfig) SpanExporter(ctx context.Context) (trace.SpanExporter, []trace.TracerProviderOption, error) {
	opts, err := e.spanOpts()
	if err != nil {
		return nil, nil, fmt.Errorf("build conn options: %w", err)
	}

	v := otlptracegrpc.NewUnstarted(opts...)

	p := e.Queue.BuildSpanProcessor(v)
	return v, []trace.TracerProviderOption{trace.WithSpanProcessor(p)}, nil
}

func (e ExporterConfig) spanOpts() ([]otlptracegrpc.Option, error) {
	opts := []otlptracegrpc.Option{}

	if e.TLS == nil {
		// Default TLS config will be used.
	} else if e.TLS.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else if c, err := e.TLS.Build(); err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	} else {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(c)))
	}

	if opts_, err := e.dialOpts(); err != nil {
		return nil, fmt.Errorf("build dial options: %w", err)
	} else if len(opts_) > 0 {
		opts = append(opts, otlptracegrpc.WithDialOption(opts_...))
	}

	if e.Endpoint != "" {
		opts = append(opts, otlptracegrpc.WithEndpoint(e.Endpoint))
	}
	if e.Compression != "" {
		opts = append(opts, otlptracegrpc.WithCompressor(e.Compression))
	}

	return opts, nil
}

func (e ExporterConfig) MetricExporter(ctx context.Context) (metric.Exporter, []metric.Option, error) {
	opts, err := e.metricOpts()
	if err != nil {
		return nil, nil, fmt.Errorf("build conn options: %w", err)
	}

	v, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create gRPC metric exporter: %w", err)
	}

	return v, []metric.Option{metric.WithReader(metric.NewPeriodicReader(v))}, nil
}

func (e ExporterConfig) metricOpts() ([]otlpmetricgrpc.Option, error) {
	opts := []otlpmetricgrpc.Option{}

	if e.TLS == nil {
		// Default TLS config will be used.
	} else if e.TLS.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	} else if c, err := e.TLS.Build(); err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	} else {
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(c)))
	}

	if opts_, err := e.dialOpts(); err != nil {
		return nil, fmt.Errorf("build dial options: %w", err)
	} else if len(opts_) > 0 {
		opts = append(opts, otlpmetricgrpc.WithDialOption(opts_...))
	}

	if e.Endpoint != "" {
		opts = append(opts, otlpmetricgrpc.WithEndpoint(e.Endpoint))
	}
	if e.Compression != "" {
		opts = append(opts, otlpmetricgrpc.WithCompressor(e.Compression))
	}

	return opts, nil
}

func (e ExporterConfig) LogExporter(ctx context.Context) (log.Exporter, []log.LoggerProviderOption, error) {
	opts, err := e.logOpts()
	if err != nil {
		return nil, nil, fmt.Errorf("build conn options: %w", err)
	}

	v, err := otlploggrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create gRPC log exporter: %w", err)
	}

	p := e.Queue.BuildLogProcessor(v)
	return v, []log.LoggerProviderOption{log.WithProcessor(p)}, nil
}

func (e ExporterConfig) logOpts() ([]otlploggrpc.Option, error) {
	opts := []otlploggrpc.Option{}

	if e.TLS == nil {
		// Default TLS config will be used.
	} else if e.TLS.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	} else if c, err := e.TLS.Build(); err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	} else {
		opts = append(opts, otlploggrpc.WithTLSCredentials(credentials.NewTLS(c)))
	}

	if opts_, err := e.dialOpts(); err != nil {
		return nil, fmt.Errorf("build dial options: %w", err)
	} else if len(opts_) > 0 {
		opts = append(opts, otlploggrpc.WithDialOption(opts_...))
	}

	if e.Endpoint != "" {
		opts = append(opts, otlploggrpc.WithEndpoint(e.Endpoint))
	}
	if e.Compression != "" {
		opts = append(opts, otlploggrpc.WithCompressor(e.Compression))
	}

	return opts, nil
}

func (e ExporterConfig) dialOpts() ([]grpc.DialOption, error) {
	opts := []grpc.DialOption{}
	if e.ReadBufferSize > 0 {
		opts = append(opts, grpc.WithReadBufferSize(e.ReadBufferSize))
	}
	if e.WriteBufferSize > 0 {
		opts = append(opts, grpc.WithWriteBufferSize(e.WriteBufferSize))
	}
	if e.Authority != "" {
		opts = append(opts, grpc.WithAuthority(e.Authority))
	}

	return opts, nil
}

type KeepaliveConfig struct {
	Time                time.Duration `yaml:"time"`
	Timeout             time.Duration `yaml:"timeout"`
	PermitWithoutStream bool          `yaml:"permit_without_stream,omitempty"`
}

func init() {
	mkot.DefaultExporterRegistry.Set("otlp", func() mkot.ExporterConfig {
		return &ExporterConfig{}
	})
}
