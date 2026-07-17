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

	// Ratio is the sampling probability in [0,1] for "trace_id_ratio" and for
	// the root sampler of "parent_based".
	Ratio float64 `yaml:"ratio,omitempty"`
}

func (c *Sampler) TracerOpts(ctx context.Context) ([]trace.TracerProviderOption, error) {
	s, err := c.build()
	if err != nil {
		return nil, err
	}
	return []trace.TracerProviderOption{trace.WithSampler(s)}, nil
}

func (c *Sampler) build() (trace.Sampler, error) {
	switch c.Type {
	case "", "always_on":
		return trace.AlwaysSample(), nil
	case "always_off":
		return trace.NeverSample(), nil
	case "trace_id_ratio":
		return trace.TraceIDRatioBased(c.Ratio), nil
	case "parent_based":
		// Respect an upstream sampling decision; fall back to the ratio at the
		// root of a new trace.
		return trace.ParentBased(trace.TraceIDRatioBased(c.Ratio)), nil
	default:
		return nil, fmt.Errorf("unknown sampler type %q (want always_on, always_off, trace_id_ratio, or parent_based)", c.Type)
	}
}

func init() {
	DefaultProcessorRegistry.Set("sampler", func() ProcessorConfig {
		return &Sampler{}
	})
}
