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
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
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

	// Protocol selects the transport: "grpc" (default) or "http" (aka
	// "http/protobuf"). Mirrors the collector's OTLP protocol selection. The
	// gRPC-only knobs (keepalive, read/write buffers, wait_for_ready,
	// balancer_name, authority, reconnection_period) are rejected under http.
	Protocol string `yaml:"protocol,omitempty"`

	// The target to which the exporter is going to send traces or metrics.
	// For grpc the syntax is https://github.com/grpc/grpc/blob/master/doc/naming.md;
	// for http it is a base URL. A scheme (http://, https://) is honored and
	// http:// implies insecure.
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

	// ReconnectionPeriod is the minimum time between gRPC connection attempts.
	// Zero uses the SDK default. Not part of the collector schema.
	ReconnectionPeriod time.Duration `yaml:"reconnection_period,omitempty"`

	// ServiceConfig is a raw gRPC service config JSON (load balancing, method
	// retry/timeout policy). Mutually exclusive with balancer_name, which is a
	// shorthand for the load-balancing fragment. protocol grpc only.
	ServiceConfig string `yaml:"service_config,omitempty"`

	// ProxyURL routes HTTP exports through the given proxy (e.g.
	// "http://proxy:3128"). Empty uses the standard HTTP_PROXY/HTTPS_PROXY
	// environment. Mirrors confighttp.proxy_url. protocol http only; gRPC honors
	// the environment proxy but has no per-client override.
	ProxyURL string `yaml:"proxy_url,omitempty"`

	// Auth applies a credential (bearer / basic / oauth2) to every export,
	// beyond the static Headers. See [AuthConfig].
	Auth *AuthConfig `yaml:"auth,omitempty"`

	// // Middlewares for the gRPC client.
	// Middlewares []configmiddleware.Config `yaml:"middlewares,omitempty"`

	// Timeout is the maximum duration for each export attempt. Zero uses the SDK
	// default (10s). Maps the collector exporterhelper `timeout`.
	//
	// Only the metric path can exceed 30s: its reader deadline is aligned with
	// this value, but the trace and log batch processors wrap every export in
	// their own 30s deadline, so a larger timeout is capped there (a
	// sending_queue disabled into a simple processor is not). Raising that
	// ceiling needs an export-timeout option on
	// [mkot.QueueConfig.BuildSpanProcessor]/BuildLogProcessor.
	Timeout time.Duration `yaml:"timeout,omitempty"`

	Retry mkot.RetryConfig `yaml:"retry_on_failure,omitempty"`
	Queue mkot.QueueConfig `yaml:"sending_queue,omitempty"`

	// Interval is the metric push period. Zero uses the SDK default (60s).
	// Not part of the collector schema (the SDK owns the push cadence).
	Interval time.Duration `yaml:"interval,omitempty"`

	// Temporality selects the metric aggregation temporality: "cumulative"
	// (default), "delta", or "lowmemory". Not part of the collector schema.
	Temporality string `yaml:"temporality,omitempty"`

	// ExemplarFilter selects which measurements may become exemplars:
	// "trace_based" (SDK default when unset), "always_on", or "always_off".
	// Not part of the collector schema.
	ExemplarFilter string `yaml:"exemplar_filter,omitempty"`
}

func (e ExporterConfig) SpanExporter(ctx context.Context) (trace.SpanExporter, []trace.TracerProviderOption, error) {
	// Unstarted: [mkot.Resolver.Start] is the single starter.
	v, err := e.newSpanExporter(ctx)
	if err != nil {
		return nil, nil, err
	}

	p, err := e.Queue.BuildSpanProcessor(v)
	if err != nil {
		return nil, nil, err
	}
	return mkot.SpanComponent(v, p), []trace.TracerProviderOption{trace.WithSpanProcessor(p)}, nil
}

func (e ExporterConfig) spanOpts() ([]otlptracegrpc.Option, error) {
	opts := []otlptracegrpc.Option{}

	if err := e.checkDurations(); err != nil {
		return nil, err
	}
	if err := e.checkEndpointTLS(); err != nil {
		return nil, err
	}
	if err := e.checkGRPCTransport(); err != nil {
		return nil, err
	}
	if e.TLS == nil {
		// Default TLS config will be used.
	} else if e.TLS.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else if c, err := e.TLS.Build(); err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	} else {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(c)))
	}

	if opts_, err := e.traceDialOpts(); err != nil {
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
	if h, err := e.headers(); err != nil {
		return nil, err
	} else if h != nil {
		opts = append(opts, otlptracegrpc.WithHeaders(h))
	}
	if e.Timeout > 0 {
		opts = append(opts, otlptracegrpc.WithTimeout(e.Timeout))
	}
	if e.ReconnectionPeriod > 0 {
		opts = append(opts, otlptracegrpc.WithReconnectionPeriod(e.ReconnectionPeriod))
	}
	if e.ServiceConfig != "" {
		opts = append(opts, otlptracegrpc.WithServiceConfig(e.ServiceConfig))
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

// MetricExporter returns the raw OTLP metric exporter for callers that push
// pre-built metricdata directly (e.g. replaying recorded data with historical
// timestamps) by calling its Export. The caller owns its lifecycle and must
// Shutdown it. No reader is installed: the returned options carry only
// MeterProvider-level settings (the exemplar filter), so [mkot.Resolver] wraps
// the exporter in its own periodic reader, and a caller that wants a ready-made
// reader should use [ExporterConfig.MetricReader] instead. Building a
// MeterProvider from these options alone would export nothing, by design.
func (e ExporterConfig) MetricExporter(ctx context.Context) (metric.Exporter, []metric.Option, error) {
	// Validate before connecting: a rejected exemplar_filter must not leak a
	// live exporter on every attempt.
	mopts, err := e.meterProviderOpts()
	if err != nil {
		return nil, nil, err
	}

	v, err := e.newMetricExporter(ctx)
	if err != nil {
		return nil, nil, err
	}
	return v, mopts, nil
}

// MetricReader wires a periodic OTLP push. The reader is the lifecycle
// component: its Shutdown flushes the final collection before closing the
// exporter.
func (e ExporterConfig) MetricReader(ctx context.Context) (metric.Reader, []metric.Option, error) {
	// Validate before connecting: a rejected exemplar_filter must not leak a
	// live exporter on every attempt.
	mopts, err := e.meterProviderOpts()
	if err != nil {
		return nil, nil, err
	}

	v, err := e.newMetricExporter(ctx)
	if err != nil {
		return nil, nil, err
	}

	r := metric.NewPeriodicReader(v, e.readerOpts()...)
	return r, append([]metric.Option{metric.WithReader(r)}, mopts...), nil
}

// readerOpts maps the metric knobs onto the periodic reader.
func (e ExporterConfig) readerOpts() []metric.PeriodicReaderOption {
	opts := []metric.PeriodicReaderOption{}
	if e.Interval > 0 {
		opts = append(opts, metric.WithInterval(e.Interval))
	}
	if e.Timeout > 0 {
		// Keep the reader's collect+export deadline aligned with the per-export
		// timeout so a raised timeout is not cancelled early by the reader's
		// 30s default.
		opts = append(opts, metric.WithTimeout(e.Timeout))
	}
	return opts
}

func (e ExporterConfig) metricOpts() ([]otlpmetricgrpc.Option, error) {
	opts := []otlpmetricgrpc.Option{}

	if err := e.checkDurations(); err != nil {
		return nil, err
	}
	if err := e.checkEndpointTLS(); err != nil {
		return nil, err
	}
	if err := e.checkGRPCTransport(); err != nil {
		return nil, err
	}
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
	if h, err := e.headers(); err != nil {
		return nil, err
	} else if h != nil {
		opts = append(opts, otlpmetricgrpc.WithHeaders(h))
	}
	if e.Timeout > 0 {
		opts = append(opts, otlpmetricgrpc.WithTimeout(e.Timeout))
	}
	if e.ReconnectionPeriod > 0 {
		opts = append(opts, otlpmetricgrpc.WithReconnectionPeriod(e.ReconnectionPeriod))
	}
	if e.ServiceConfig != "" {
		opts = append(opts, otlpmetricgrpc.WithServiceConfig(e.ServiceConfig))
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
	case "lowmemory":
		// Delta for sync counters and histograms (which the SDK need not retain
		// between cycles), cumulative for everything else — the spec's
		// memory-optimized preference.
		opts = append(opts, otlpmetricgrpc.WithTemporalitySelector(metric.LowMemoryTemporalitySelector))
	default:
		return nil, fmt.Errorf("unknown temporality %q (want cumulative, delta, or lowmemory)", e.Temporality)
	}

	return opts, nil
}

func (e ExporterConfig) LogExporter(ctx context.Context) (log.Exporter, []log.LoggerProviderOption, error) {
	v, err := e.newLogExporter(ctx)
	if err != nil {
		return nil, nil, err
	}

	p, err := e.Queue.BuildLogProcessor(v)
	if err != nil {
		// The exporter is already live (an open gRPC ClientConn); a rejected
		// sending_queue must not leak it on every construction attempt.
		_ = v.Shutdown(ctx)
		return nil, nil, err
	}
	return mkot.LogComponent(v, p), []log.LoggerProviderOption{log.WithProcessor(p)}, nil
}

func (e ExporterConfig) logOpts() ([]otlploggrpc.Option, error) {
	opts := []otlploggrpc.Option{}

	if err := e.checkDurations(); err != nil {
		return nil, err
	}
	if err := e.checkEndpointTLS(); err != nil {
		return nil, err
	}
	if err := e.checkGRPCTransport(); err != nil {
		return nil, err
	}
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
	if h, err := e.headers(); err != nil {
		return nil, err
	} else if h != nil {
		opts = append(opts, otlploggrpc.WithHeaders(h))
	}
	if e.Timeout > 0 {
		opts = append(opts, otlploggrpc.WithTimeout(e.Timeout))
	}
	if e.ReconnectionPeriod > 0 {
		opts = append(opts, otlploggrpc.WithReconnectionPeriod(e.ReconnectionPeriod))
	}
	if e.ServiceConfig != "" {
		opts = append(opts, otlploggrpc.WithServiceConfig(e.ServiceConfig))
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

// keepaliveMinTime is grpc-go internal.KeepaliveMinPingTime: a shorter client
// ping interval is silently clamped up to this value.
const keepaliveMinTime = 10 * time.Second

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
	if e.Keepalive != nil {
		// Only wire keepalive with an explicit ping interval: grpc clamps Time=0
		// up to its 10s minimum, which would silently ENABLE pings for an empty
		// block. A timeout/permit_without_stream set without time is a partial
		// config that would otherwise be dropped whole — reject it instead.
		// (A negative time is already rejected by checkDurations.)
		if e.Keepalive.Time <= 0 {
			if e.Keepalive.Timeout != 0 || e.Keepalive.PermitWithoutStream {
				return nil, fmt.Errorf("keepalive: time must be set when timeout or permit_without_stream is configured")
			}
		} else if e.Keepalive.Time < keepaliveMinTime {
			// grpc silently clamps a shorter interval up to its 10s minimum, so a
			// configured 1s would run at 10s with no diagnostic. Reject it.
			return nil, fmt.Errorf("keepalive: time must be at least %s (grpc clamps shorter intervals), got %s", keepaliveMinTime, e.Keepalive.Time)
		} else {
			opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:                e.Keepalive.Time,
				Timeout:             e.Keepalive.Timeout,
				PermitWithoutStream: e.Keepalive.PermitWithoutStream,
			}))
		}
	}
	if e.WaitForReady {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.WaitForReady(true)))
	}
	if e.BalancerName != "" {
		opts = append(opts, grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingConfig": [{%q: {}}]}`, e.BalancerName)))
	}
	if e.Auth != nil {
		if err := e.Auth.validate(); err != nil {
			return nil, err
		}
		opts = append(opts, grpc.WithPerRPCCredentials(e.Auth.perRPCCredentials()))
	}

	return opts, nil
}

// traceDialOpts re-seeds the User-Agent the trace exporter would otherwise lose.
// otlptracegrpc.WithDialOption REPLACES the SDK's seeded dial options (including
// its "OTel OTLP Exporter Go/<ver>" identifier), so setting an unrelated dial
// knob (buffers, keepalive, authority, balancer) would strip the header backends
// key on.
//
// Only traces: otlpmetricgrpc and otlploggrpc APPEND to their own seed instead,
// and grpc's WithUserAgent is last-wins, so injecting here would overwrite their
// correct per-signal identifiers with the trace exporter's.
func (e ExporterConfig) traceDialOpts() ([]grpc.DialOption, error) {
	opts, err := e.dialOpts()
	if err != nil || len(opts) == 0 {
		return opts, err
	}
	return append([]grpc.DialOption{grpc.WithUserAgent("OTel OTLP Exporter Go/" + otlptrace.Version())}, opts...), nil
}

// endpointScheme returns the endpoint's URL scheme, or "" when the endpoint
// carries none (a bare host:port, or a schemeless gRPC target such as
// "unix-abstract:otel").
func (e ExporterConfig) endpointScheme() (string, error) {
	if !strings.Contains(e.Endpoint, "://") {
		return "", nil
	}
	u, err := url.Parse(e.Endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid endpoint URL %q: %w", e.Endpoint, err)
	}
	return u.Scheme, nil
}

// endpointHasScheme reports whether the gRPC builders must use WithEndpointURL.
// An http/https endpoint (e.g. "https://collector:4317", the ubiquitous
// OTEL_EXPORTER_OTLP_ENDPOINT / collector form) must: WithEndpoint expects a
// bare host:port and would otherwise dial the whole URL string as a literal
// gRPC target and never connect. WithEndpointURL also makes http:// imply
// insecure, matching collector/SDK ergonomics.
//
// Any other scheme is a gRPC target — "dns://", "unix://", "xds://",
// "passthrough://" — that grpc's own resolver understands, so it is handed to
// WithEndpoint untouched. Rejecting those would make a sidecar's unix socket
// inexpressible, and the Endpoint doc comment points at the gRPC naming spec
// that defines them.
func (e ExporterConfig) endpointHasScheme() (bool, error) {
	s, err := e.endpointScheme()
	if err != nil {
		return false, err
	}
	return s == "http" || s == "https", nil
}

// checkDurations rejects negative durations. Every knob treats 0 as "use the
// SDK default", so a negative value would silently take the same path as unset
// instead of the setting the user asked for.
func (e ExporterConfig) checkDurations() error {
	for _, d := range []struct {
		name string
		v    time.Duration
	}{
		{"timeout", e.Timeout},
		{"interval", e.Interval},
		{"reconnection_period", e.ReconnectionPeriod},
	} {
		if d.v < 0 {
			return fmt.Errorf("%s must not be negative, got %s", d.name, d.v)
		}
	}
	if e.Keepalive != nil {
		if e.Keepalive.Time < 0 {
			return fmt.Errorf("keepalive: time must not be negative, got %s", e.Keepalive.Time)
		}
		if e.Keepalive.Timeout < 0 {
			return fmt.Errorf("keepalive: timeout must not be negative, got %s", e.Keepalive.Timeout)
		}
	}
	return nil
}

// checkGRPCTransport rejects config that is meaningless or contradictory on the
// gRPC transport: proxy_url is an HTTP-only knob (gRPC honors the environment
// proxy), and service_config is the full JSON that balancer_name is a shorthand
// for, so setting both is ambiguous.
func (e ExporterConfig) checkGRPCTransport() error {
	if e.ProxyURL != "" {
		return fmt.Errorf("proxy_url is not supported with protocol grpc (it honors HTTP_PROXY/HTTPS_PROXY from the environment)")
	}
	if e.ServiceConfig != "" && e.BalancerName != "" {
		return fmt.Errorf("service_config and balancer_name are mutually exclusive")
	}
	return nil
}

// checkEndpointTLS rejects an endpoint scheme that contradicts the tls block.
// The SDK resolves gRPC credentials by priority, not by option order: any
// WithTLSCredentials wins over the Insecure that WithEndpointURL derives from
// http://, so an http:// endpoint plus a non-insecure tls block would silently
// keep TLS on and never reach a plaintext collector. The reverse pairing
// (https:// with tls.insecure) is contradictory in the other direction, and the
// scheme wins because it is applied last. protocol: http already rejects the
// first pairing; reject both here rather than resolving them silently.
func (e ExporterConfig) checkEndpointTLS() error {
	if e.TLS == nil {
		return nil
	}
	s, err := e.endpointScheme()
	if err != nil {
		return err
	}
	if s == "http" && !e.TLS.Insecure {
		return fmt.Errorf("insecure http:// endpoint cannot use a tls client configuration (set tls.insecure, or use https://)")
	}
	if s == "https" && e.TLS.Insecure {
		return fmt.Errorf("https:// endpoint contradicts tls.insecure (drop the scheme, or use http://)")
	}
	return nil
}

// httpEndpointHasScheme mirrors [ExporterConfig.endpointHasScheme] for
// protocol: http, where a gRPC target scheme carries no meaning and must be
// rejected rather than dialed as a hostname.
func (e ExporterConfig) httpEndpointHasScheme() (bool, error) {
	s, err := e.endpointScheme()
	if err != nil {
		return false, err
	}
	switch s {
	case "":
		return false, nil
	case "http", "https":
		return true, nil
	default:
		return false, fmt.Errorf("unsupported endpoint scheme %q for protocol http (want http or https)", s)
	}
}

// meterProviderOpts returns MeterProvider-level options not tied to the reader
// (currently the exemplar filter). An empty selection leaves the SDK default
// (trace_based).
func (e ExporterConfig) meterProviderOpts() ([]metric.Option, error) {
	switch e.ExemplarFilter {
	case "":
		return nil, nil
	case "always_on":
		return []metric.Option{metric.WithExemplarFilter(exemplar.AlwaysOnFilter)}, nil
	case "always_off":
		return []metric.Option{metric.WithExemplarFilter(exemplar.AlwaysOffFilter)}, nil
	case "trace_based":
		return []metric.Option{metric.WithExemplarFilter(exemplar.TraceBasedFilter)}, nil
	default:
		return nil, fmt.Errorf("unknown exemplar_filter %q (want always_on, always_off, or trace_based)", e.ExemplarFilter)
	}
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
		return "", fmt.Errorf("unsupported compression %q (only gzip is supported)", e.Compression)
	}
}

// headers flattens the opaque name/value list for the exporter options. A
// duplicate name is rejected rather than silently overwritten (last-wins),
// restoring the distinct-names invariant the MapList type documents.
//
// Names are compared and stored lowercased because header names are
// case-insensitive on both transports: net/http canonicalizes them (so one of
// "Authorization"/"authorization" would win at random through map iteration),
// while gRPC metadata lowercases and appends (so both would ride as one
// two-valued header). Neither is what a duplicate name means.
func (e ExporterConfig) headers() (map[string]string, error) {
	if len(e.Headers) == 0 {
		return nil, nil
	}
	m := make(map[string]string, len(e.Headers))
	for name, value := range e.Headers.Iter {
		k := strings.ToLower(name)
		if _, dup := m[k]; dup {
			return nil, fmt.Errorf("headers: duplicate name %q", name)
		}
		m[k] = string(value)
	}
	return m, nil
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
		return retryPolicy{}, false, fmt.Errorf("retry_on_failure: randomization_factor and multiplier are not supported (the OTLP exporter's backoff factors are fixed)")
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
