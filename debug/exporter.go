package debug

import (
	"context"
	"fmt"
	"io"

	"github.com/lesomnus/mkot"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

var _ mkot.ExporterConfig = (*ExporterConfig)(nil)

type ExporterConfig struct {
	mkot.UnimplementedExporterConfig

	// OutputPaths is a list of file paths to write logging output to.
	// This option can only be used when use_internal_logger is false.
	// Special strings "stdout" and "stderr" are interpreted as os.Stdout and os.Stderr respectively.
	// All other values are treated as file paths.
	// If not set, defaults to ["stdout"].
	OutputPaths []string `yaml:"output_paths,omitempty"`

	Queue mkot.QueueConfig `yaml:"sending_queue,omitempty"`
}

// queue is the effective queue config: debug output is meant to be read live,
// so an entirely-unset sending_queue means synchronous output rather than
// QueueConfig's batch-by-default. Any explicit setting is an opt-in and is
// honored as-is.
func (e ExporterConfig) queue() mkot.QueueConfig {
	c := e.Queue
	untouched := c.Enabled == nil && c.NumConsumers == 0 && !c.WaitForResult &&
		!c.BlockOnOverflow && c.QueueSize == 0 && c.Batch == (mkot.BatchConfig{})
	if untouched {
		disabled := false
		c.Enabled = &disabled
	}
	return c
}

func (e ExporterConfig) SpanExporter(ctx context.Context) (trace.SpanExporter, []trace.TracerProviderOption, error) {
	w, err := e.open()
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	v, err := stdouttrace.New(stdouttrace.WithWriter(w))
	if err != nil {
		return nil, nil, err
	}

	p := e.queue().BuildSpanProcessor(v)
	return mkot.SpanComponent(v, p), []trace.TracerProviderOption{trace.WithSpanProcessor(p)}, nil
}

// MetricExporter returns the raw stdout metric exporter for callers that push
// pre-built metricdata directly (e.g. replaying recorded data with historical
// timestamps) instead of sampling instruments through a reader.
func (e ExporterConfig) MetricExporter(ctx context.Context) (metric.Exporter, []metric.Option, error) {
	w, err := e.open()
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	v, err := stdoutmetric.New(stdoutmetric.WithWriter(w))
	if err != nil {
		return nil, nil, err
	}

	return v, []metric.Option{metric.WithReader(metric.NewPeriodicReader(v))}, nil
}

func (e ExporterConfig) MetricReader(ctx context.Context) (metric.Reader, []metric.Option, error) {
	w, err := e.open()
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	v, err := stdoutmetric.New(stdoutmetric.WithWriter(w))
	if err != nil {
		return nil, nil, err
	}

	// The reader is the lifecycle component: its Shutdown flushes the final
	// collection before closing the exporter.
	r := metric.NewPeriodicReader(v)
	return r, []metric.Option{metric.WithReader(r)}, nil
}

func (e ExporterConfig) LogExporter(ctx context.Context) (log.Exporter, []log.LoggerProviderOption, error) {
	w, err := e.open()
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	v, err := stdoutlog.New(stdoutlog.WithWriter(w))
	if err != nil {
		return nil, nil, err
	}

	p := e.queue().BuildLogProcessor(v)
	return mkot.LogComponent(v, p), []log.LoggerProviderOption{log.WithProcessor(p)}, nil
}

func (e ExporterConfig) open() (io.WriteCloser, error) {
	ps := e.OutputPaths
	if len(ps) == 0 {
		ps = []string{"stdout"}
	}

	ws, err := mkot.Outputs.OpenAll(ps)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	return ws, nil
}

func init() {
	mkot.DefaultExporterRegistry.Set("debug", func() mkot.ExporterConfig {
		return &ExporterConfig{}
	})
}
