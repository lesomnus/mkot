package mkot_test

import (
	"context"
	"sync"
	"testing"

	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	"go.opentelemetry.io/otel/sdk/trace"
)

type recordingSpanExporter struct {
	mu    sync.Mutex
	spans int
}

func (e *recordingSpanExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans += len(spans)
	return nil
}

func (e *recordingSpanExporter) Shutdown(context.Context) error { return nil }

func (e *recordingSpanExporter) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spans
}

// A zero-valued QueueConfig builds a batcher with the SDK defaults; it must not
// pass the zero sizes through (a zero max-queue batcher drops every span).
func TestBuildSpanProcessorDefaultsDeliver(t *testing.T) {
	ctx, x := x.New(t)
	rec := &recordingSpanExporter{}
	p := mkot.QueueConfig{}.BuildSpanProcessor(rec)
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(p))
	_, span := tp.Tracer("t").Start(ctx, "s")
	span.End()
	x.NoError(p.Shutdown(context.Background()))
	x.Eq(1, rec.count())
}

func TestBuildSpanProcessorDisabledIsSynchronous(t *testing.T) {
	ctx, x := x.New(t)
	rec := &recordingSpanExporter{}
	disabled := false
	p := mkot.QueueConfig{Enabled: &disabled}.BuildSpanProcessor(rec)
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(p))
	_, span := tp.Tracer("t").Start(ctx, "s")
	span.End()
	x.Eq(1, rec.count()) // simple processor exports on End, no flush needed
}
