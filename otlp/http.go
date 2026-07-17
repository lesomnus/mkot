package otlp

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

const (
	protocolGRPC = "grpc"
	protocolHTTP = "http"
)

// protocol normalizes the configured transport; empty defaults to grpc.
func (e ExporterConfig) protocol() (string, error) {
	switch e.Protocol {
	case "", "grpc":
		return protocolGRPC, nil
	case "http", "http/protobuf":
		return protocolHTTP, nil
	default:
		return "", fmt.Errorf("unknown protocol %q (want grpc or http/protobuf)", e.Protocol)
	}
}

// rejectGRPCOnly errors if a gRPC-only knob is set under protocol http, so a
// value HTTP cannot honor is not silently dropped.
func (e ExporterConfig) rejectGRPCOnly() error {
	switch {
	case e.Keepalive != nil:
		return fmt.Errorf("keepalive is not supported with protocol http")
	case e.ReadBufferSize != 0:
		return fmt.Errorf("read_buffer_size is not supported with protocol http")
	case e.WriteBufferSize != 0:
		return fmt.Errorf("write_buffer_size is not supported with protocol http")
	case e.WaitForReady:
		return fmt.Errorf("wait_for_ready is not supported with protocol http")
	case e.BalancerName != "":
		return fmt.Errorf("balancer_name is not supported with protocol http")
	case e.Authority != "":
		return fmt.Errorf("authority is not supported with protocol http")
	case e.ReconnectionPeriod != 0:
		return fmt.Errorf("reconnection_period is not supported with protocol http")
	}
	return nil
}

func (e ExporterConfig) newSpanExporter(ctx context.Context) (trace.SpanExporter, error) {
	p, err := e.protocol()
	if err != nil {
		return nil, err
	}
	if p == protocolHTTP {
		opts, err := e.spanHTTPOpts()
		if err != nil {
			return nil, fmt.Errorf("build conn options: %w", err)
		}
		return otlptracehttp.NewUnstarted(opts...), nil
	}
	opts, err := e.spanOpts()
	if err != nil {
		return nil, fmt.Errorf("build conn options: %w", err)
	}
	return otlptracegrpc.NewUnstarted(opts...), nil
}

func (e ExporterConfig) newMetricExporter(ctx context.Context) (metric.Exporter, error) {
	p, err := e.protocol()
	if err != nil {
		return nil, err
	}
	if p == protocolHTTP {
		opts, err := e.metricHTTPOpts()
		if err != nil {
			return nil, fmt.Errorf("build conn options: %w", err)
		}
		v, err := otlpmetrichttp.New(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("create HTTP metric exporter: %w", err)
		}
		return v, nil
	}
	opts, err := e.metricOpts()
	if err != nil {
		return nil, fmt.Errorf("build conn options: %w", err)
	}
	v, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gRPC metric exporter: %w", err)
	}
	return v, nil
}

func (e ExporterConfig) newLogExporter(ctx context.Context) (log.Exporter, error) {
	p, err := e.protocol()
	if err != nil {
		return nil, err
	}
	if p == protocolHTTP {
		opts, err := e.logHTTPOpts()
		if err != nil {
			return nil, fmt.Errorf("build conn options: %w", err)
		}
		v, err := otlploghttp.New(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("create HTTP log exporter: %w", err)
		}
		return v, nil
	}
	opts, err := e.logOpts()
	if err != nil {
		return nil, fmt.Errorf("build conn options: %w", err)
	}
	v, err := otlploggrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gRPC log exporter: %w", err)
	}
	return v, nil
}

// spanHTTPOpts builds the OTLP/HTTP trace exporter options from the shared
// config (TLS, endpoint, compression, headers, timeout, retry). Endpoint scheme
// handling and compression/header validation are shared with the gRPC path.
func (e ExporterConfig) spanHTTPOpts() ([]otlptracehttp.Option, error) {
	if err := e.rejectGRPCOnly(); err != nil {
		return nil, err
	}
	opts := []otlptracehttp.Option{}

	if e.TLS == nil {
		// Default TLS config will be used.
	} else if e.TLS.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	} else if c, err := e.TLS.Build(); err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	} else {
		opts = append(opts, otlptracehttp.WithTLSClientConfig(c))
	}

	if e.Endpoint != "" {
		if scheme, err := e.endpointHasScheme(); err != nil {
			return nil, err
		} else if scheme {
			opts = append(opts, otlptracehttp.WithEndpointURL(e.Endpoint))
		} else {
			opts = append(opts, otlptracehttp.WithEndpoint(e.Endpoint))
		}
	}
	if c, err := e.compressor(); err != nil {
		return nil, err
	} else if c == "gzip" {
		opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	}
	if h, err := e.headers(); err != nil {
		return nil, err
	} else if h != nil {
		opts = append(opts, otlptracehttp.WithHeaders(h))
	}
	if e.Timeout > 0 {
		opts = append(opts, otlptracehttp.WithTimeout(e.Timeout))
	}
	if p, ok, err := e.retryPolicy(); err != nil {
		return nil, err
	} else if ok {
		opts = append(opts, otlptracehttp.WithRetry(otlptracehttp.RetryConfig{
			Enabled:         p.enabled,
			InitialInterval: p.initial,
			MaxInterval:     p.max,
			MaxElapsedTime:  p.elapsed,
		}))
	}
	return opts, nil
}

func (e ExporterConfig) metricHTTPOpts() ([]otlpmetrichttp.Option, error) {
	if err := e.rejectGRPCOnly(); err != nil {
		return nil, err
	}
	opts := []otlpmetrichttp.Option{}

	if e.TLS == nil {
		// Default TLS config will be used.
	} else if e.TLS.Insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	} else if c, err := e.TLS.Build(); err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	} else {
		opts = append(opts, otlpmetrichttp.WithTLSClientConfig(c))
	}

	if e.Endpoint != "" {
		if scheme, err := e.endpointHasScheme(); err != nil {
			return nil, err
		} else if scheme {
			opts = append(opts, otlpmetrichttp.WithEndpointURL(e.Endpoint))
		} else {
			opts = append(opts, otlpmetrichttp.WithEndpoint(e.Endpoint))
		}
	}
	if c, err := e.compressor(); err != nil {
		return nil, err
	} else if c == "gzip" {
		opts = append(opts, otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
	}
	if h, err := e.headers(); err != nil {
		return nil, err
	} else if h != nil {
		opts = append(opts, otlpmetrichttp.WithHeaders(h))
	}
	if e.Timeout > 0 {
		opts = append(opts, otlpmetrichttp.WithTimeout(e.Timeout))
	}
	if p, ok, err := e.retryPolicy(); err != nil {
		return nil, err
	} else if ok {
		opts = append(opts, otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{
			Enabled:         p.enabled,
			InitialInterval: p.initial,
			MaxInterval:     p.max,
			MaxElapsedTime:  p.elapsed,
		}))
	}
	switch e.Temporality {
	case "", "cumulative":
	case "delta":
		opts = append(opts, otlpmetrichttp.WithTemporalitySelector(metric.DeltaTemporalitySelector))
	case "lowmemory":
		opts = append(opts, otlpmetrichttp.WithTemporalitySelector(metric.LowMemoryTemporalitySelector))
	default:
		return nil, fmt.Errorf("unknown temporality %q (want cumulative, delta, or lowmemory)", e.Temporality)
	}
	return opts, nil
}

func (e ExporterConfig) logHTTPOpts() ([]otlploghttp.Option, error) {
	if err := e.rejectGRPCOnly(); err != nil {
		return nil, err
	}
	opts := []otlploghttp.Option{}

	if e.TLS == nil {
		// Default TLS config will be used.
	} else if e.TLS.Insecure {
		opts = append(opts, otlploghttp.WithInsecure())
	} else if c, err := e.TLS.Build(); err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	} else {
		opts = append(opts, otlploghttp.WithTLSClientConfig(c))
	}

	if e.Endpoint != "" {
		if scheme, err := e.endpointHasScheme(); err != nil {
			return nil, err
		} else if scheme {
			opts = append(opts, otlploghttp.WithEndpointURL(e.Endpoint))
		} else {
			opts = append(opts, otlploghttp.WithEndpoint(e.Endpoint))
		}
	}
	if c, err := e.compressor(); err != nil {
		return nil, err
	} else if c == "gzip" {
		opts = append(opts, otlploghttp.WithCompression(otlploghttp.GzipCompression))
	}
	if h, err := e.headers(); err != nil {
		return nil, err
	} else if h != nil {
		opts = append(opts, otlploghttp.WithHeaders(h))
	}
	if e.Timeout > 0 {
		opts = append(opts, otlploghttp.WithTimeout(e.Timeout))
	}
	if p, ok, err := e.retryPolicy(); err != nil {
		return nil, err
	} else if ok {
		opts = append(opts, otlploghttp.WithRetry(otlploghttp.RetryConfig{
			Enabled:         p.enabled,
			InitialInterval: p.initial,
			MaxInterval:     p.max,
			MaxElapsedTime:  p.elapsed,
		}))
	}
	return opts, nil
}
