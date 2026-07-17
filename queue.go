package mkot

// See https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/exporterhelper/README.md#sending-queue

import (
	"fmt"
	"time"

	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/trace"
)

// QueueConfig defines configuration for queueing and batching incoming requests.
type QueueConfig struct {
	Enabled *bool `yaml:",omitempty"`

	// NumConsumers is the maximum number of concurrent consumers from the queue.
	// This applies across all different optional configurations from above (e.g. wait_for_result, block_on_overflow, storage, etc.).
	NumConsumers int `yaml:"num_consumers,omitempty"`

	// WaitForResult determines if incoming requests are blocked until the request is processed or not.
	// Currently, this option is not available when persistent queue is configured using the storage configuration.
	WaitForResult bool `yaml:"wait_for_result,omitempty"`

	// BlockOnOverflow determines the behavior when the component's TotalSize limit is reached.
	// If true, the component will wait for space; otherwise, operations will immediately return a retryable error.
	BlockOnOverflow bool `yaml:"block_on_overflow,omitempty"`

	// // Sizer determines the type of size measurement used by this component.
	// // It accepts "requests", "items", or "bytes".
	// Sizer request.SizerType `yaml:"sizer,omitempty"`
	// QueueSize represents the maximum data size allowed for concurrent storage and processing.
	QueueSize int64 `yaml:"queue_size,omitempty"`

	// // StorageID if not empty, enables the persistent storage and uses the component specified
	// // as a storage extension for the persistent queue.
	// // TODO: This will be changed to Optional when available.
	// // See https://github.com/open-telemetry/opentelemetry-collector/issues/13822
	// StorageID *component.ID `yaml:"storage,omitempty"`
	// BatchConfig it configures how the requests are consumed from the queue and batch together during consumption.
	Batch BatchConfig `yaml:"batch,omitempty"`
}

func (c QueueConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// rejectUnsupported errors on sending_queue knobs the SDK batch processors
// cannot honor, instead of silently dropping them (matching the reject-don't-
// drop precedent used for unsupported retry knobs). block_on_overflow maps to
// trace.WithBlocking for spans but has no analogue in the log batch processor,
// so it is rejected on the log path only.
func (c QueueConfig) rejectUnsupported(isLog bool) error {
	if c.NumConsumers != 0 {
		return fmt.Errorf("sending_queue: num_consumers is not supported (the SDK batch processor is single-consumer)")
	}
	if c.WaitForResult {
		return fmt.Errorf("sending_queue: wait_for_result is not supported (the SDK batch processor is asynchronous)")
	}
	if c.Batch.MinSize != 0 {
		return fmt.Errorf("sending_queue: batch.min_size is not supported (the SDK has no minimum-batch-size)")
	}
	if isLog && c.BlockOnOverflow {
		return fmt.Errorf("sending_queue: block_on_overflow is not supported for logs (the SDK log batch processor drops on overflow)")
	}
	return nil
}

func (c QueueConfig) BuildSpanProcessor(v trace.SpanExporter) (trace.SpanProcessor, error) {
	if !c.IsEnabled() {
		return trace.NewSimpleSpanProcessor(v), nil
	}
	if err := c.rejectUnsupported(false); err != nil {
		return nil, err
	}

	// Unset (zero) values must keep the SDK defaults: the trace batcher does
	// not clamp them, and a zero max-queue batcher silently drops every span.
	opts := []trace.BatchSpanProcessorOption{}
	if c.QueueSize > 0 {
		opts = append(opts, trace.WithMaxQueueSize(int(c.QueueSize)))
	}
	if c.Batch.FlushTimeout > 0 {
		opts = append(opts, trace.WithBatchTimeout(c.Batch.FlushTimeout))
	}
	if c.Batch.MaxSize > 0 {
		opts = append(opts, trace.WithMaxExportBatchSize(int(c.Batch.MaxSize)))
	}
	if c.BlockOnOverflow {
		// Block the producer instead of dropping spans when the queue is full.
		opts = append(opts, trace.WithBlocking())
	}
	return trace.NewBatchSpanProcessor(v, opts...), nil
}

func (c QueueConfig) BuildLogProcessor(v log.Exporter) (log.Processor, error) {
	if !c.IsEnabled() {
		return log.NewSimpleProcessor(v), nil
	}
	if err := c.rejectUnsupported(true); err != nil {
		return nil, err
	}

	opts := []log.BatchProcessorOption{}
	if c.QueueSize > 0 {
		opts = append(opts, log.WithMaxQueueSize(int(c.QueueSize)))
	}
	if c.Batch.FlushTimeout > 0 {
		opts = append(opts, log.WithExportInterval(c.Batch.FlushTimeout))
	}
	if c.Batch.MaxSize > 0 {
		opts = append(opts, log.WithExportMaxBatchSize(int(c.Batch.MaxSize)))
	}
	return log.NewBatchProcessor(v, opts...), nil
}

// BatchConfig defines a configuration for batching requests based on a timeout and a minimum number of items.
type BatchConfig struct {
	// FlushTimeout sets the time after which a batch will be sent regardless of its size.
	FlushTimeout time.Duration `yaml:"flush_timeout,omitempty"`

	// MinSize defines the configuration for the minimum size of a batch.
	MinSize int64 `yaml:"min_size,omitempty"`

	// MaxSize defines the configuration for the maximum size of a batch.
	MaxSize int64 `yaml:"max_size,omitempty"`

	// // Sizer determines the type of size measurement used by the batch.
	// // If not configured, use the same configuration as the queue.
	// // It accepts "requests", "items", or "bytes".
	// Sizer request.SizerType `yaml:"sizer,omitempty"`

	// // Partition defines the partitioning of the batches configuration.
	// Partition PartitionConfig `yaml:"partition,omitempty"`
}
