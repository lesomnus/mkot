package mkot

// See https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/exporterhelper/README.md#sending-queue

import (
	"time"

	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/trace"
)

// QueueConfig defines configuration for queueing and batching incoming requests.
type QueueConfig struct {
	Enabled bool

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

func (c QueueConfig) BuildSpanProcessor(v trace.SpanExporter) trace.SpanProcessor {
	if !c.Enabled {
		return trace.NewSimpleSpanProcessor(v)
	}
	return trace.NewBatchSpanProcessor(v,
		trace.WithMaxQueueSize(int(c.QueueSize)),
		trace.WithBatchTimeout(c.Batch.FlushTimeout),
		trace.WithMaxExportBatchSize(int(c.Batch.MaxSize)),
	)
}

func (c QueueConfig) BuildLogProcessor(v log.Exporter) log.Processor {
	if !c.Enabled {
		return log.NewSimpleProcessor(v)
	}

	return log.NewBatchProcessor(v,
		log.WithMaxQueueSize(int(c.QueueSize)),
		log.WithExportInterval(c.Batch.FlushTimeout),
		log.WithExportMaxBatchSize(int(c.Batch.MaxSize)),
	)
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
