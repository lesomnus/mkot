package mkot_test

import (
	"math"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	"go.opentelemetry.io/otel/sdk/trace"
	otrace "go.opentelemetry.io/otel/trace"
)

func ratio(v float64) *float64 { return &v }

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
	x.Eq(false, sampled(&mkot.Sampler{Type: "trace_id_ratio", Ratio: ratio(0)}))
	x.Eq(true, sampled(&mkot.Sampler{Type: "trace_id_ratio", Ratio: ratio(1)}))

	if _, err := (&mkot.Sampler{Type: "nope"}).TracerOpts(ctx); err == nil {
		t.Fatal("unknown sampler type must error")
	}
}

// parent_based without a ratio must keep the traces this service roots: reading
// the unset ratio as 0 silently dropped 100% of them.
func TestSamplerParentBased(t *testing.T) {
	ctx, x := x.New(t)

	rootSampled := func(s *mkot.Sampler) bool {
		opts, err := s.TracerOpts(ctx)
		x.NoError(err)
		tp := trace.NewTracerProvider(opts...)
		_, span := tp.Tracer("t").Start(ctx, "root")
		defer span.End()
		return span.SpanContext().IsSampled()
	}

	x.Eq(true, rootSampled(&mkot.Sampler{Type: "parent_based"}))
	x.Eq(true, rootSampled(&mkot.Sampler{Type: "parent_based", Ratio: ratio(1)}))
	// An explicit zero stays honored: root nothing, but follow a sampled parent.
	x.Eq(false, rootSampled(&mkot.Sampler{Type: "parent_based", Ratio: ratio(0)}))

	opts, err := (&mkot.Sampler{Type: "parent_based", Ratio: ratio(0)}).TracerOpts(ctx)
	x.NoError(err)
	tp := trace.NewTracerProvider(opts...)
	parent := otrace.ContextWithSpanContext(ctx, otrace.NewSpanContext(otrace.SpanContextConfig{
		TraceID:    otrace.TraceID{0x01},
		SpanID:     otrace.SpanID{0x01},
		TraceFlags: otrace.FlagsSampled,
		Remote:     true,
	}))
	_, span := tp.Tracer("t").Start(parent, "child")
	defer span.End()
	x.Eq(true, span.SpanContext().IsSampled())
}

func TestSamplerRejectsBadRatio(t *testing.T) {
	ctx, _ := x.New(t)

	for _, c := range []*mkot.Sampler{
		{Type: "trace_id_ratio"},                            // ratio required
		{Type: "trace_id_ratio", Ratio: ratio(1.5)},         // out of range
		{Type: "trace_id_ratio", Ratio: ratio(-0.1)},        // out of range
		{Type: "trace_id_ratio", Ratio: ratio(math.NaN())},  // NaN samples everything
		{Type: "parent_based", Ratio: ratio(math.NaN())},    // ditto at the root
		{Type: "trace_id_ratio", Ratio: ratio(math.Inf(1))}, // +Inf
		{Type: "always_on", Ratio: ratio(0.1)},              // ratio does not apply
		{Ratio: ratio(0.1)},                                 // ditto, default type
	} {
		if _, err := c.TracerOpts(ctx); err == nil {
			t.Fatalf("%+v must error", c)
		}
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
	x.NotNil(s.Ratio)
	x.Eq(0.1, *s.Ratio)
}
