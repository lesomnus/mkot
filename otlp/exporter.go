package otlp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/opaque"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

var _ mkot.ExporterConfig = (*ExporterConfig)(nil)

// ExporterConfig defines common settings for a gRPC client configuration.
type ExporterConfig struct {
	mkot.UnimplementedExporterConfig

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

	// Timeout is the maximum duration for each export attempt. Zero uses the SDK
	// default (10s). Maps the collector exporterhelper `timeout`.
	Timeout time.Duration `yaml:"timeout,omitempty"`

	Retry mkot.RetryConfig `yaml:"retry_on_failure,omitempty"`
	Queue mkot.QueueConfig `yaml:"sending_queue,omitempty"`

	// Interval is the metric push period. Zero uses the SDK default (60s).
	// Not part of the collector schema (the SDK owns the push cadence).
	Interval time.Duration `yaml:"interval,omitempty"`

	// Temporality selects the metric aggregation temporality: "cumulative"
	// (default) or "delta". Not part of the collector schema.
	Temporality string `yaml:"temporality,omitempty"`
}

func (e ExporterConfig) SpanExporter(ctx context.Context) (trace.SpanExporter, []trace.TracerProviderOption, error) {
	opts, err := e.spanOpts()
	if err != nil {
		return nil, nil, fmt.Errorf("build conn options: %w", err)
	}

	// Unstarted: [mkot.Resolver.Start] is the single starter.
	v := otlptracegrpc.NewUnstarted(opts...)

	p, err := e.Queue.BuildSpanProcessor(v)
	if err != nil {
		return nil, nil, err
	}
	return mkot.SpanComponent(v, p), []trace.TracerProviderOption{trace.WithSpanProcessor(p)}, nil
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
		if scheme, err := e.endpointHasScheme(); err != nil {
			return nil, err
		} else if scheme {
			opts = append(opts, otlptracegrpc.WithEndpointURL(e.Endpoint))
		} else {
			opts = append(opts, otlptracegrpc.WithEndpoint(e.Endpoint))
		}
	}
	if c, err := e.compressor(); err != nil {
		return nil, err
	} else if c != "" {
		opts = append(opts, otlptracegrpc.WithCompressor(c))
	}
	if h := e.headers(); h != nil {
		opts = append(opts, otlptracegrpc.WithHeaders(h))
	}
	if e.Timeout > 0 {
		opts = append(opts, otlptracegrpc.WithTimeout(e.Timeout))
	}
	p, ok, err := e.retryPolicy()
	if err != nil {
		return nil, err
	}
	if ok {
		opts = append(opts, otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         p.enabled,
			InitialInterval: p.initial,
			MaxInterval:     p.max,
			MaxElapsedTime:  p.elapsed,
		}))
	}

	return opts, nil
}

// MetricExporter returns the raw OTLP/gRPC metric exporter for callers that
// push pre-built metricdata directly (e.g. replaying recorded data with
// historical timestamps) instead of sampling instruments through a reader. The
// caller owns its lifecycle and must Shutdown it.
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

// MetricReader wires a periodic OTLP push. The reader is the lifecycle
// component: its Shutdown flushes the final collection before closing the
// exporter.
func (e ExporterConfig) MetricReader(ctx context.Context) (metric.Reader, []metric.Option, error) {
	opts, err := e.metricOpts()
	if err != nil {
		return nil, nil, fmt.Errorf("build conn options: %w", err)
	}

	v, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create gRPC metric exporter: %w", err)
	}

	ropts := []metric.PeriodicReaderOption{}
	if e.Interval > 0 {
		ropts = append(ropts, metric.WithInterval(e.Interval))
	}
	if e.Timeout > 0 {
		// Keep the reader's collect+export deadline aligned with the per-export
		// timeout so a raised timeout is not cancelled early by the reader's
		// 30s default.
		ropts = append(ropts, metric.WithTimeout(e.Timeout))
	}
	r := metric.NewPeriodicReader(v, ropts...)
	return r, []metric.Option{metric.WithReader(r)}, nil
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
		if scheme, err := e.endpointHasScheme(); err != nil {
			return nil, err
		} else if scheme {
			opts = append(opts, otlpmetricgrpc.WithEndpointURL(e.Endpoint))
		} else {
			opts = append(opts, otlpmetricgrpc.WithEndpoint(e.Endpoint))
		}
	}
	if c, err := e.compressor(); err != nil {
		return nil, err
	} else if c != "" {
		opts = append(opts, otlpmetricgrpc.WithCompressor(c))
	}
	if h := e.headers(); h != nil {
		opts = append(opts, otlpmetricgrpc.WithHeaders(h))
	}
	if e.Timeout > 0 {
		opts = append(opts, otlpmetricgrpc.WithTimeout(e.Timeout))
	}
	p, ok, err := e.retryPolicy()
	if err != nil {
		return nil, err
	}
	if ok {
		opts = append(opts, otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{
			Enabled:         p.enabled,
			InitialInterval: p.initial,
			MaxInterval:     p.max,
			MaxElapsedTime:  p.elapsed,
		}))
	}

	switch e.Temporality {
	case "", "cumulative":
	case "delta":
		// The SDK's spec-standard delta preference: counters and histograms go
		// delta, everything else (gauges, up-down counters) stays cumulative — a
		// delta gauge would vanish from exports unless re-recorded every cycle.
		opts = append(opts, otlpmetricgrpc.WithTemporalitySelector(metric.DeltaTemporalitySelector))
	default:
		return nil, fmt.Errorf("unknown temporality %q (want cumulative or delta)", e.Temporality)
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

	p, err := e.Queue.BuildLogProcessor(v)
	if err != nil {
		return nil, nil, err
	}
	return mkot.LogComponent(v, p), []log.LoggerProviderOption{log.WithProcessor(p)}, nil
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
		if scheme, err := e.endpointHasScheme(); err != nil {
			return nil, err
		} else if scheme {
			opts = append(opts, otlploggrpc.WithEndpointURL(e.Endpoint))
		} else {
			opts = append(opts, otlploggrpc.WithEndpoint(e.Endpoint))
		}
	}
	if c, err := e.compressor(); err != nil {
		return nil, err
	} else if c != "" {
		opts = append(opts, otlploggrpc.WithCompressor(c))
	}
	if h := e.headers(); h != nil {
		opts = append(opts, otlploggrpc.WithHeaders(h))
	}
	if e.Timeout > 0 {
		opts = append(opts, otlploggrpc.WithTimeout(e.Timeout))
	}
	p, ok, err := e.retryPolicy()
	if err != nil {
		return nil, err
	}
	if ok {
		opts = append(opts, otlploggrpc.WithRetry(otlploggrpc.RetryConfig{
			Enabled:         p.enabled,
			InitialInterval: p.initial,
			MaxInterval:     p.max,
			MaxElapsedTime:  p.elapsed,
		}))
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
	if e.Keepalive != nil && e.Keepalive.Time > 0 {
		// Only with an explicit ping interval: grpc clamps Time=0 up to its 10s
		// minimum, which would silently ENABLE keepalive pings for an empty block.
		opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                e.Keepalive.Time,
			Timeout:             e.Keepalive.Timeout,
			PermitWithoutStream: e.Keepalive.PermitWithoutStream,
		}))
	}
	if e.WaitForReady {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.WaitForReady(true)))
	}
	if e.BalancerName != "" {
		opts = append(opts, grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingConfig": [{%q: {}}]}`, e.BalancerName)))
	}

	if len(opts) == 0 {
		return opts, nil
	}
	// These are passed through otlp*grpc.WithDialOption, which REPLACES the SDK's
	// seeded dial options (including its "OTel OTLP Exporter Go/<ver>" User-Agent).
	// Re-seed the identifier so setting an unrelated dial knob (buffers, keepalive,
	// authority, balancer) does not strip the header backends key on.
	return append([]grpc.DialOption{grpc.WithUserAgent("OTel OTLP Exporter Go/" + otlptrace.Version())}, opts...), nil
}

// endpointHasScheme reports whether the endpoint carries an http/https scheme.
// A scheme-bearing endpoint (e.g. "https://collector:4317", the ubiquitous
// OTEL_EXPORTER_OTLP_ENDPOINT / collector form) must go through WithEndpointURL:
// WithEndpoint expects a bare host:port and would otherwise dial the whole URL
// string as a literal gRPC target and never connect. WithEndpointURL also makes
// http:// imply insecure, matching collector/SDK ergonomics.
func (e ExporterConfig) endpointHasScheme() (bool, error) {
	if !strings.Contains(e.Endpoint, "://") {
		return false, nil
	}
	u, err := url.Parse(e.Endpoint)
	if err != nil {
		return false, fmt.Errorf("invalid endpoint URL %q: %w", e.Endpoint, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false, fmt.Errorf("unsupported endpoint scheme %q (want http or https)", u.Scheme)
	}
	return true, nil
}

// compressor validates the configured compression and returns the gRPC
// compressor name to use ("" means none). Only gzip is registered by the
// SDK/grpc; any other value (zstd, snappy, zlib, ...) would be accepted here,
// silently sent UNCOMPRESSED, and then fail at export time with "Compressor is
// not installed", so reject it up front instead of dropping the intent.
func (e ExporterConfig) compressor() (string, error) {
	switch e.Compression {
	case "", "none":
		return "", nil
	case "gzip":
		return "gzip", nil
	default:
		return "", fmt.Errorf("unsupported compression %q (only gzip is supported by the OTLP gRPC exporter)", e.Compression)
	}
}

// headers flattens the opaque name/value list for the exporter options.
func (e ExporterConfig) headers() map[string]string {
	if len(e.Headers) == 0 {
		return nil
	}
	m := make(map[string]string, len(e.Headers))
	for name, value := range e.Headers.Iter {
		m[name] = string(value)
	}
	return m
}

type retryPolicy struct {
	enabled bool
	initial time.Duration
	max     time.Duration
	elapsed time.Duration
}

// retryPolicy maps retry_on_failure onto the exporter retry settings; ok=false
// when the config is untouched so the exporter defaults stay in effect.
func (e ExporterConfig) retryPolicy() (retryPolicy, bool, error) {
	c := e.Retry
	// The OTel SDK exporters cannot express these knobs (their backoff factors
	// are fixed); reject rather than silently drop the tuning.
	if c.RandomizationFactor != 0 || c.Multiplier != 0 {
		return retryPolicy{}, false, fmt.Errorf("retry_on_failure: randomization_factor and multiplier are not supported by the OTLP gRPC exporter")
	}
	if c.Enabled == nil && c.InitialInterval == 0 && c.MaxInterval == 0 && c.MaxElapsedTime == 0 {
		return retryPolicy{}, false, nil
	}
	p := retryPolicy{
		enabled: c.IsEnabled(),
		initial: c.InitialInterval,
		max:     c.MaxInterval,
		elapsed: c.MaxElapsedTime, // 0 = never stop retrying, as documented
	}
	// A partial config must not produce zero backoff intervals (hot retry loop).
	if p.initial <= 0 {
		p.initial = 5 * time.Second
	}
	if p.max <= 0 {
		p.max = 30 * time.Second
	}
	return p, true, nil
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
