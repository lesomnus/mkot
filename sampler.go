package mkot

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/sdk/trace"
)

// Sampler is a processor that installs a head sampler on the tracer provider.
// Without it every span is AlwaysSample; it lets a config cut trace volume/cost
// (e.g. sample 10% of new traces) the way the SDK trace.WithSampler does.
type Sampler struct {
	UnimplementedProcessorConfig `yaml:"-"`

	// Type selects the sampler: "always_on" (default), "always_off",
	// "trace_id_ratio", or "parent_based".
	Type string `yaml:"type,omitempty"`

	// Ratio is the sampling probability in [0,1]. It is required by
	// "trace_id_ratio" and optional for "parent_based". A pointer so that an
	// explicit `ratio: 0` ("root nothing") stays distinct from an unset ratio.
	Ratio *float64 `yaml:"ratio,omitempty"`
}

func (c *Sampler) TracerOpts(ctx context.Context) ([]trace.TracerProviderOption, error) {
	s, err := c.build()
	if err != nil {
		return nil, err
	}
	return []trace.TracerProviderOption{trace.WithSampler(s)}, nil
}

func (c *Sampler) build() (trace.Sampler, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	switch c.Type {
	case "", "always_on":
		return trace.AlwaysSample(), nil
	case "always_off":
		return trace.NeverSample(), nil
	case "trace_id_ratio":
		// Required: an unset ratio would be 0, which drops every trace.
		if c.Ratio == nil {
			return nil, fmt.Errorf("sampler: ratio is required for %q", c.Type)
		}
		return trace.TraceIDRatioBased(*c.Ratio), nil
	case "parent_based":
		// Respect an upstream sampling decision. An unset ratio keeps the traces
		// this service roots (the spec's parentbased_always_on); reading it as a
		// zero ratio would silently drop all of them.
		root := trace.AlwaysSample()
		if c.Ratio != nil {
			root = trace.TraceIDRatioBased(*c.Ratio)
		}
		return trace.ParentBased(root), nil
	default:
		return nil, fmt.Errorf("unknown sampler type %q (want always_on, always_off, trace_id_ratio, or parent_based)", c.Type)
	}
}

func (c *Sampler) validate() error {
	if c.Ratio == nil {
		return nil
	}
	// The SDK clamps out-of-range fractions silently; reject instead.
	if *c.Ratio < 0 || *c.Ratio > 1 {
		return fmt.Errorf("sampler: ratio %v is out of range [0,1]", *c.Ratio)
	}
	// always_on/always_off ignore the ratio: a config that reads like it samples
	// a fraction but always samples must not pass silently.
	switch c.Type {
	case "trace_id_ratio", "parent_based":
		return nil
	default:
		return fmt.Errorf("sampler: ratio does not apply to type %q (want trace_id_ratio or parent_based)", c.Type)
	}
}

func init() {
	DefaultProcessorRegistry.Set("sampler", func() ProcessorConfig {
		return &Sampler{}
	})
}
