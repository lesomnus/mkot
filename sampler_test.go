package mkot_test

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	"go.opentelemetry.io/otel/sdk/trace"
)

func TestSampler(t *testing.T) {
	ctx, x := x.New(t)

	sampled := func(s *mkot.Sampler) bool {
		opts, err := s.TracerOpts(ctx)
		x.NoError(err)
		tp := trace.NewTracerProvider(opts...)
		_, span := tp.Tracer("t").Start(ctx, "s")
		defer span.End()
		return span.SpanContext().IsSampled()
	}

	x.Eq(true, sampled(&mkot.Sampler{Type: "always_on"}))
	x.Eq(true, sampled(&mkot.Sampler{})) // default is always_on
	x.Eq(false, sampled(&mkot.Sampler{Type: "always_off"}))
	x.Eq(false, sampled(&mkot.Sampler{Type: "trace_id_ratio", Ratio: 0}))
	x.Eq(true, sampled(&mkot.Sampler{Type: "trace_id_ratio", Ratio: 1}))

	if _, err := (&mkot.Sampler{Type: "nope"}).TracerOpts(ctx); err == nil {
		t.Fatal("unknown sampler type must error")
	}
}

func TestSamplerDecodes(t *testing.T) {
	_, x := x.New(t)
	const src = `
processors:
  sampler:
    type: trace_id_ratio
    ratio: 0.1
`
	var c mkot.Config
	x.NoError(yaml.Unmarshal([]byte(src), &c))
	s, ok := c.Processors[mkot.Id("sampler")].(*mkot.Sampler)
	if !ok {
		t.Fatalf("expected *mkot.Sampler, got %T", c.Processors[mkot.Id("sampler")])
	}
	x.Eq("trace_id_ratio", s.Type)
	x.Eq(0.1, s.Ratio)
}
