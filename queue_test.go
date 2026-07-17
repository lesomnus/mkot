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
	p, err := mkot.QueueConfig{}.BuildSpanProcessor(rec)
	x.NoError(err)
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
	p, err := mkot.QueueConfig{Enabled: &disabled}.BuildSpanProcessor(rec)
	x.NoError(err)
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(p))
	_, span := tp.Tracer("t").Start(ctx, "s")
	span.End()
	x.Eq(1, rec.count()) // simple processor exports on End, no flush needed
}

// block_on_overflow maps to a blocking span processor; the knobs the SDK cannot
// express are rejected rather than silently dropped.
func TestBuildProcessorQueueKnobs(t *testing.T) {
	_, x := x.New(t)
	rec := &recordingSpanExporter{}

	// block_on_overflow is expressible for spans (trace.WithBlocking).
	_, err := mkot.QueueConfig{BlockOnOverflow: true}.BuildSpanProcessor(rec)
	x.NoError(err)

	// Inexpressible span knobs must error.
	for _, c := range []mkot.QueueConfig{
		{NumConsumers: 4},
		{WaitForResult: true},
		{Batch: mkot.BatchConfig{MinSize: 100}},
	} {
		if _, err := c.BuildSpanProcessor(rec); err == nil {
			t.Fatalf("expected an error for %+v", c)
		}
	}

	// Logs cannot block on overflow, so that too must error.
	if _, err := (mkot.QueueConfig{BlockOnOverflow: true}).BuildLogProcessor(nil); err == nil {
		t.Fatal("block_on_overflow must error on the log path")
	}
}
