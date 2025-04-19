package mkot_test

import (
	"testing"
	"time"

	"github.com/lesomnus/mkot"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/yaml.v3"
)

func TestConfigUnmarshalYAML(t *testing.T) {
	require := require.New(t)
	const Raw = `
enabled: true
processors:
  batcher:
    max_queue_size: 1
    max_export_batch_size: 2
    export_buffer_size: 3

    batch_timeout: 1m
    export_timeout: 2m
    export_interval: 3m

  batcher/foo:
    max_queue_size: 42

  resource:
    attributes:
      - key: string
        value: foo
      - key: int
        value: 42
      - key: float
        value: 3.14
      - key: bool
        value: true

      - key: strings
        value: [foo, bar]
      - key: ints
        value: [1, 2]
      - key: floats
        value: [3.14, 2.71]
      - key: bools
        value: [true, false, true]

    detectors:
      - os
      - process

  periodic_reader:
    interval: 1m
    timeout: 2m

exporters:
  debug:

  otlp:
    endpoint: example.com:80
    compression: gzip
    headers:
      foo: bar
      baz: qux

providers:
  default:
    processors: [batcher/foo, resource]
    exporters: [debug, otlp]
`

	c := mkot.Config{}
	err := yaml.Unmarshal([]byte(Raw), &c)
	require.NoError(err)
	require.Contains(c.Processors, mkot.Id("batcher"))
	require.Contains(c.Processors, mkot.Id("resource"))
	require.Contains(c.Processors, mkot.Id("periodic_reader"))

	batcher_v := c.Processors["batcher"]
	require.IsType(&mkot.BatcherConfig{}, batcher_v)

	batcher := batcher_v.(*mkot.BatcherConfig)
	require.NotNil(batcher)
	require.NotNil(batcher.MaxQueueSize)
	require.Equal(1, *batcher.MaxQueueSize)
	require.NotNil(batcher.MaxExportBatchSize)
	require.Equal(2, *batcher.MaxExportBatchSize)
	require.NotNil(batcher.ExportBufferSize)
	require.Equal(3, *batcher.ExportBufferSize)
	require.NotNil(batcher.BatchTimeout)
	require.Equal(1*time.Minute, *batcher.BatchTimeout)
	require.NotNil(batcher.ExportTimeout)
	require.Equal(2*time.Minute, *batcher.ExportTimeout)
	require.NotNil(batcher.ExportInterval)
	require.Equal(3*time.Minute, *batcher.ExportInterval)

	batcher_foo_v := c.Processors["batcher/foo"]
	require.IsType(&mkot.BatcherConfig{}, batcher_foo_v)

	batcher_foo := batcher_foo_v.(*mkot.BatcherConfig)
	require.NotNil(batcher_foo)
	require.NotNil(batcher_foo.MaxQueueSize)
	require.Equal(42, *batcher_foo.MaxQueueSize)

	resource_v := c.Processors["resource"]
	require.IsType(&mkot.ResourceConfig{}, resource_v)

	resource := resource_v.(*mkot.ResourceConfig)
	require.NotNil(resource)
	require.Equal([]mkot.Attribute{
		{
			Key:   "string",
			Value: attribute.StringValue("foo"),
		},
		{
			Key:   "int",
			Value: attribute.IntValue(42),
		},
		{
			Key:   "float",
			Value: attribute.Float64Value(3.14),
		},
		{
			Key:   "bool",
			Value: attribute.BoolValue(true),
		},
		{
			Key:   "strings",
			Value: attribute.StringSliceValue([]string{"foo", "bar"}),
		},
		{
			Key:   "ints",
			Value: attribute.IntSliceValue([]int{1, 2}),
		},
		{
			Key:   "floats",
			Value: attribute.Float64SliceValue([]float64{3.14, 2.71}),
		},
		{
			Key:   "bools",
			Value: attribute.BoolSliceValue([]bool{true, false, true}),
		},
	}, resource.Attributes)
	require.Equal([]string{"os", "process"}, resource.Detectors)

	periodic_reader_v := c.Processors["periodic_reader"]
	require.IsType(&mkot.PeriodicReaderConfig{}, periodic_reader_v)

	periodic_reader := periodic_reader_v.(*mkot.PeriodicReaderConfig)
	require.NotNil(periodic_reader)
	require.NotNil(periodic_reader.Interval)
	require.Equal(1*time.Minute, *periodic_reader.Interval)
	require.NotNil(periodic_reader.Timeout)
	require.Equal(2*time.Minute, *periodic_reader.Timeout)

	debug_exporter_v := c.Exporters["debug"]
	require.IsType(&mkot.DebugExporterConfig{}, debug_exporter_v)

	debug_exporter := debug_exporter_v.(*mkot.DebugExporterConfig)
	require.NotNil(debug_exporter)

	otlp_exporter_v := c.Exporters["otlp"]
	require.IsType(&mkot.OtlpExporterConfig{}, otlp_exporter_v)

	otlp_exporter := otlp_exporter_v.(*mkot.OtlpExporterConfig)
	require.NotNil(otlp_exporter)
	require.Equal("example.com:80", otlp_exporter.Endpoint)
	require.NotNil(otlp_exporter.Compression)
	require.Equal("gzip", *otlp_exporter.Compression)
	require.NotNil(otlp_exporter.Headers)
	require.Equal(map[string]string{
		"foo": "bar",
		"baz": "qux",
	}, otlp_exporter.Headers)

	default_provider := c.Providers["default"]
	require.Equal([]mkot.Id{"batcher/foo", "resource"}, default_provider.Processors)
	require.Equal([]mkot.Id{"debug", "otlp"}, default_provider.Exporters)
}
