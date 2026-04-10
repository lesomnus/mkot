package mkot

import (
	"time"

	"github.com/lesomnus/mkot/internal/z"
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
	opts = z.Take(opts, c.MaxQueueSize, trace.WithMaxQueueSize)
	opts = z.Take(opts, c.MaxExportBatchSize, trace.WithMaxExportBatchSize)
	opts = z.Take(opts, c.BatchTimeout, trace.WithBatchTimeout)
	opts = z.Take(opts, c.ExportTimeout, trace.WithExportTimeout)
	opts = z.Enable(opts, c.Blocking, trace.WithBlocking)

	return opts
}

func (c *Batch) LogOpts() []log.BatchProcessorOption {
	opts := []log.BatchProcessorOption{}
	opts = z.Take(opts, c.MaxQueueSize, log.WithMaxQueueSize)
	opts = z.Take(opts, c.MaxExportBatchSize, log.WithExportMaxBatchSize)
	opts = z.Take(opts, c.ExportBufferSize, log.WithExportBufferSize)
	opts = z.Take(opts, c.ExportTimeout, log.WithExportTimeout)
	opts = z.Take(opts, c.ExportInterval, log.WithExportInterval)

	return opts
}
