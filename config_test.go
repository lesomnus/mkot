package mkot_test

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	"go.opentelemetry.io/otel/attribute"
)

func TestConfigUnmarshalYAML(t *testing.T) {
	_, x := x.New(t)
	const Raw = `
enabled: true
processors:
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
        value: [a, b, c]
      - key: ints
        value: [1, 2, 3]
      - key: floats
        value: [1.1, 2.2, 3.3]
      - key: bools
        value: [true, false, true]

    detectors:
      - os
      - process

providers:
  tracer:
    processors:
      - resource
`

	c := mkot.Config{}
	err := yaml.Unmarshal([]byte(Raw), &c)
	x.NoError(err)
	x.Contains(c.Processors, mkot.Id("resource"))

	resource_v := c.Processors["resource"]
	resource := mkot.Resource{}
	x.TypeAs(resource_v, &resource)
	x.Eq(
		mkot.Resource{
			Attributes: []mkot.Attr{
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
					Value: attribute.StringSliceValue([]string{"a", "b", "c"}),
				},
				{
					Key:   "ints",
					Value: attribute.IntSliceValue([]int{1, 2, 3}),
				},
				{
					Key:   "floats",
					Value: attribute.Float64SliceValue([]float64{1.1, 2.2, 3.3}),
				},
				{
					Key:   "bools",
					Value: attribute.BoolSliceValue([]bool{true, false, true}),
				},
			},
			Detectors: []string{"os", "process"},
		},
		resource,
	)

	// default_provider := c.Providers["tracer"]
	// require.Equal([]mkot.Id{"resource"}, default_provider.Processors)
	// require.Equal([]mkot.Id{"debug"}, default_provider.Exporters)
}

// A collector-style service.pipelines block maps onto mkot providers: traces →
// tracer, metrics → meter, logs → logger (with the /name suffix preserved), and
// the receivers key is accepted but ignored.
func TestServicePipelinesAlias(t *testing.T) {
	_, x := x.New(t)
	const src = `
processors:
  resource:
    attributes:
      - key: service.name
        value: svc
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [resource]
    metrics/internal:
      processors: [resource]
    logs:
      processors: [resource]
`
	var c mkot.Config
	x.NoError(yaml.Unmarshal([]byte(src), &c))
	x.Contains(c.Providers, mkot.Id("tracer"))
	x.Contains(c.Providers, mkot.Id("meter/internal"))
	x.Contains(c.Providers, mkot.Id("logger"))
	x.Eq([]mkot.Id{"resource"}, c.Providers[mkot.Id("tracer")].Processors)

	// An unknown pipeline signal is rejected.
	if err := yaml.Unmarshal([]byte("service:\n  pipelines:\n    profiles:\n      processors: []\n"), &mkot.Config{}); err == nil {
		t.Fatal("unknown pipeline signal must error")
	}
	// Defining the same provider twice (providers + service.pipelines) errors.
	dup := `
providers:
  tracer:
    processors: [resource]
service:
  pipelines:
    traces:
      processors: [resource]
`
	if err := yaml.Unmarshal([]byte(dup), &mkot.Config{}); err == nil {
		t.Fatal("a provider defined by both providers and service.pipelines must error")
	}
}
