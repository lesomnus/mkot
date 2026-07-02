package mkot

import (
	"context"

	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/trace"
)

// SpanComponent wraps a span exporter wired through a processor into the
// lifecycle component a Resolver manages: Shutdown drains the processor (which
// flushes batched spans and then closes the exporter, for both the simple and
// batch variants) instead of closing the exporter under the processor's feet,
// and Start passes through so an unstarted exporter is started by
// [Resolver.Start].
func SpanComponent(v trace.SpanExporter, p trace.SpanProcessor) trace.SpanExporter {
	return spanComponent{v, p}
}

type spanComponent struct {
	trace.SpanExporter
	p trace.SpanProcessor
}

func (c spanComponent) Start(ctx context.Context) error {
	s, ok := c.SpanExporter.(interface{ Start(context.Context) error })
	if !ok {
		return nil
	}
	return s.Start(ctx)
}

func (c spanComponent) Shutdown(ctx context.Context) error { return c.p.Shutdown(ctx) }

// LogComponent mirrors [SpanComponent] for the log pipeline: Shutdown drains
// the processor so batched records are flushed before the exporter closes.
func LogComponent(v log.Exporter, p log.Processor) log.Exporter {
	return logComponent{v, p}
}

type logComponent struct {
	log.Exporter
	p log.Processor
}

func (c logComponent) Shutdown(ctx context.Context) error { return c.p.Shutdown(ctx) }
