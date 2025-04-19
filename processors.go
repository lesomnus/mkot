package mkot

import (
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

type BatcherConfig struct {
	MaxQueueSize       *int `yaml:"max_queue_size"`
	MaxExportBatchSize *int `yaml:"max_export_batch_size"`
	ExportBufferSize   *int `yaml:"export_buffer_size"` // Logger only.

	BatchTimeout   *time.Duration `yaml:"batch_timeout"` // Tracer only.
	ExportTimeout  *time.Duration `yaml:"export_timeout"`
	ExportInterval *time.Duration `yaml:"export_interval"` // Logger only.

	Blocking bool // Tracer only.
}

func (c *BatcherConfig) handle(ctx *resolveContext) error {
	trace_opts := c.traceOpts()
	ctx.trace.is_touched = true
	for _, v := range ctx.trace.exporters {
		ctx.trace.opts = append(ctx.trace.opts, trace.WithBatcher(v, trace_opts...))
	}

	log_opts := c.logOpts()
	ctx.log.is_touched = true
	for _, v := range ctx.log.exporters {
		ctx.log.opts = append(ctx.log.opts, log.WithProcessor(log.NewBatchProcessor(v, log_opts...)))
	}

	return nil
}

func (c *BatcherConfig) traceOpts() []trace.BatchSpanProcessorOption {
	opts := []trace.BatchSpanProcessorOption{}
	opts = take(opts, c.MaxQueueSize, trace.WithMaxQueueSize)
	opts = take(opts, c.MaxExportBatchSize, trace.WithMaxExportBatchSize)
	opts = take(opts, c.BatchTimeout, trace.WithBatchTimeout)
	opts = take(opts, c.ExportTimeout, trace.WithExportTimeout)
	opts = enable(opts, c.Blocking, trace.WithBlocking)

	return opts
}

func (c *BatcherConfig) logOpts() []log.BatchProcessorOption {
	opts := []log.BatchProcessorOption{}
	opts = take(opts, c.MaxQueueSize, log.WithMaxQueueSize)
	opts = take(opts, c.MaxExportBatchSize, log.WithExportMaxBatchSize)
	opts = take(opts, c.ExportBufferSize, log.WithExportBufferSize)
	opts = take(opts, c.ExportTimeout, log.WithExportTimeout)
	opts = take(opts, c.ExportInterval, log.WithExportInterval)

	return opts
}

type ResourceConfig struct {
	Attributes []Attribute
	Detectors  []string
}

func (c *ResourceConfig) handle(ctx *resolveContext) error {
	opts := c.opts()

	v, err := resource.New(ctx, opts...)
	if err != nil {
		return fmt.Errorf("create resource: %w", err)
	}

	w, err := resource.Merge(ctx.resource, v)
	if err != nil {
		return fmt.Errorf("merge resource: %w", err)
	}

	ctx.resource = w
	return err
}

func (c *ResourceConfig) opts() []resource.Option {
	opts := []resource.Option{}

	kvs := []attribute.KeyValue{}
	for _, a := range c.Attributes {
		kvs = append(kvs, a.toKv())
	}
	if len(kvs) > 0 {
		opts = append(opts, resource.WithAttributes(kvs...))
	}

	for _, v := range c.Detectors {
		switch v {
		case "env":
			opts = append(opts, resource.WithFromEnv())

		case "container":
			opts = append(opts, resource.WithContainer())
		case "container.id":
			opts = append(opts, resource.WithContainerID())
		case "host":
			opts = append(opts, resource.WithHost())
		case "host.id":
			opts = append(opts, resource.WithHostID())
		case "os":
			opts = append(opts, resource.WithOS())
		case "os.description":
			opts = append(opts, resource.WithOSDescription())
		case "os.type":
			opts = append(opts, resource.WithOSType())
		case "process":
			opts = append(opts, resource.WithProcess())
		case "process.command_args":
			opts = append(opts, resource.WithProcessCommandArgs())
		case "process.executable.name":
			opts = append(opts, resource.WithProcessExecutableName())
		case "process.executable.path":
			opts = append(opts, resource.WithProcessExecutablePath())
		case "process.owner":
			opts = append(opts, resource.WithProcessOwner())
		case "process.pid":
			opts = append(opts, resource.WithProcessPID())
		case "process.runtime.description":
			opts = append(opts, resource.WithProcessRuntimeDescription())
		case "process.runtime.name":
			opts = append(opts, resource.WithProcessRuntimeName())
		case "process.runtime.vesion":
			opts = append(opts, resource.WithProcessRuntimeVersion())
		case "telemetry.sdk":
			opts = append(opts, resource.WithTelemetrySDK())
		}
	}

	return opts
}

// Meter only.
type PeriodicReaderConfig struct {
	Interval *time.Duration
	Timeout  *time.Duration
}

func (c *PeriodicReaderConfig) handle(ctx *resolveContext) error {
	opts := c.opts()
	for _, v := range ctx.metric.exporters {
		ctx.metric.opts = append(ctx.metric.opts, metric.WithReader(metric.NewPeriodicReader(v, opts...)))
	}

	return nil
}

func (c *PeriodicReaderConfig) opts() []metric.PeriodicReaderOption {
	opts := []metric.PeriodicReaderOption{}
	opts = take(opts, c.Interval, metric.WithInterval)
	opts = take(opts, c.Timeout, metric.WithTimeout)

	return opts
}
