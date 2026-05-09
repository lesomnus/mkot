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

func (e ExporterConfig) SpanExporter(ctx context.Context) (trace.SpanExporter, []trace.TracerProviderOption, error) {
	w, err := e.open()
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	v, err := stdouttrace.New(stdouttrace.WithWriter(w))
	if err != nil {
		return nil, nil, err
	}

	p := e.Queue.BuildSpanProcessor(v)
	return v, []trace.TracerProviderOption{trace.WithSpanProcessor(p)}, nil
}

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

func (e ExporterConfig) LogExporter(ctx context.Context) (log.Exporter, []log.LoggerProviderOption, error) {
	w, err := e.open()
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	v, err := stdoutlog.New(stdoutlog.WithWriter(w))
	if err != nil {
		return nil, nil, err
	}

	p := e.Queue.BuildLogProcessor(v)
	return v, []log.LoggerProviderOption{log.WithProcessor(p)}, nil
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
