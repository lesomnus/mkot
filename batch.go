package mkot

import (
	"time"

	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Batch struct {
	MaxQueueSize       *int `yaml:"max_queue_size"`
	MaxExportBatchSize *int `yaml:"max_export_batch_size"`
	ExportBufferSize   *int `yaml:"export_buffer_size"` // Logger only.

	BatchTimeout   *time.Duration `yaml:"batch_timeout"` // Tracer only.
	ExportTimeout  *time.Duration `yaml:"export_timeout"`
	ExportInterval *time.Duration `yaml:"export_interval"` // Logger only.

	Blocking bool // Tracer only.
}

func (c *Batch) SpanOpts() []trace.BatchSpanProcessorOption {
	opts := []trace.BatchSpanProcessorOption{}
	opts = take(opts, c.MaxQueueSize, trace.WithMaxQueueSize)
	opts = take(opts, c.MaxExportBatchSize, trace.WithMaxExportBatchSize)
	opts = take(opts, c.BatchTimeout, trace.WithBatchTimeout)
	opts = take(opts, c.ExportTimeout, trace.WithExportTimeout)
	opts = enable(opts, c.Blocking, trace.WithBlocking)

	return opts
}

func (c *Batch) LogOpts() []log.BatchProcessorOption {
	opts := []log.BatchProcessorOption{}
	opts = take(opts, c.MaxQueueSize, log.WithMaxQueueSize)
	opts = take(opts, c.MaxExportBatchSize, log.WithExportMaxBatchSize)
	opts = take(opts, c.ExportBufferSize, log.WithExportBufferSize)
	opts = take(opts, c.ExportTimeout, log.WithExportTimeout)
	opts = take(opts, c.ExportInterval, log.WithExportInterval)

	return opts
}
