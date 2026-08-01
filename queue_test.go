package mkot_test

import (
	"context"
	"sync"
	"testing"
	"time"

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

	// num_consumers: 1 is exactly what the single-consumer batch processor does.
	_, err = mkot.QueueConfig{NumConsumers: 1}.BuildSpanProcessor(rec)
	x.NoError(err)

	// Inexpressible or nonsensical span knobs must error, whether the queue is
	// enabled or not: disabling it does not make them expressible.
	disabled := false
	for _, c := range []mkot.QueueConfig{
		{NumConsumers: 4},
		{NumConsumers: -1},
		{WaitForResult: true},
		{Batch: mkot.BatchConfig{MinSize: 100}},
		{QueueSize: -1},
		{Batch: mkot.BatchConfig{MaxSize: -1}},
		{Batch: mkot.BatchConfig{FlushTimeout: -time.Second}},
		{Enabled: &disabled, NumConsumers: 4},
		{Enabled: &disabled, WaitForResult: true},
		{Enabled: &disabled, Batch: mkot.BatchConfig{MinSize: 100}},
		{Enabled: &disabled, QueueSize: -1},
	} {
		if _, err := c.BuildSpanProcessor(rec); err == nil {
			t.Fatalf("expected an error for %+v", c)
		}
	}
	if _, err := (mkot.QueueConfig{Enabled: &disabled, BlockOnOverflow: true}).BuildLogProcessor(nil); err == nil {
		t.Fatal("block_on_overflow must error on the log path even when disabled")
	}

	// Logs cannot block on overflow, so that too must error.
	if _, err := (mkot.QueueConfig{BlockOnOverflow: true}).BuildLogProcessor(nil); err == nil {
		t.Fatal("block_on_overflow must error on the log path")
	}
}

// blockingSpanExporter parks in ExportSpans until it is released, so the queue
// behind it fills up.
type blockingSpanExporter struct {
	release chan struct{}
}

func (e *blockingSpanExporter) ExportSpans(context.Context, []trace.ReadOnlySpan) error {
	<-e.release
	return nil
}

func (e *blockingSpanExporter) Shutdown(context.Context) error { return nil }

// block_on_overflow must actually block the producer: the default batcher drops
// spans when the queue is full, which is the opposite of what the config asks
// for. Asserting only that it builds cannot tell the two apart.
func TestBlockOnOverflowBlocksProducer(t *testing.T) {
	emit := func(t *testing.T, block bool) bool {
		t.Helper()
		ctx, x := x.New(t)
		exp := &blockingSpanExporter{release: make(chan struct{})}
		// Release the exporter no matter how the test ends, so Shutdown returns.
		t.Cleanup(func() { close(exp.release) })

		p, err := mkot.QueueConfig{
			QueueSize:       1,
			BlockOnOverflow: block,
			Batch:           mkot.BatchConfig{MaxSize: 1},
		}.BuildSpanProcessor(exp)
		x.NoError(err)
		tp := trace.NewTracerProvider(trace.WithSpanProcessor(p))

		done := make(chan struct{})
		go func() {
			defer close(done)
			for range 50 {
				_, span := tp.Tracer("t").Start(ctx, "s")
				span.End()
			}
		}()
		select {
		case <-done:
			return false // the producer never blocked: spans were dropped
		case <-time.After(500 * time.Millisecond):
			return true // still blocked on a full queue
		}
	}

	if emit(t, false) {
		t.Fatal("without block_on_overflow the producer must not block (spans are dropped)")
	}
	if !emit(t, true) {
		t.Fatal("block_on_overflow must block the producer instead of dropping spans")
	}
}

// deadlineSpanExporter records the deadline of the context each export runs
// under, so a test can see the batch processor's export-timeout wrapping.
type deadlineSpanExporter struct {
	mu       sync.Mutex
	deadline time.Time
	ok       bool
	done     chan struct{}
}

func (e *deadlineSpanExporter) ExportSpans(ctx context.Context, _ []trace.ReadOnlySpan) error {
	e.mu.Lock()
	e.deadline, e.ok = ctx.Deadline()
	e.mu.Unlock()
	select {
	case <-e.done:
	default:
		close(e.done)
	}
	return nil
}

func (e *deadlineSpanExporter) Shutdown(context.Context) error { return nil }

// ExportTimeout must reach the batch processor's export deadline: without it the
// SDK caps every export at its 30s default, silently truncating a larger
// configured timeout.
func TestExportTimeoutLiftsTheCeiling(t *testing.T) {
	budget := func(t *testing.T, timeout time.Duration) time.Duration {
		t.Helper()
		ctx, x := x.New(t)
		exp := &deadlineSpanExporter{done: make(chan struct{})}
		p, err := mkot.QueueConfig{ExportTimeout: timeout}.BuildSpanProcessor(exp)
		x.NoError(err)
		tp := trace.NewTracerProvider(trace.WithSpanProcessor(p))
		_, span := tp.Tracer("t").Start(ctx, "s")
		span.End()
		x.NoError(tp.ForceFlush(ctx))
		select {
		case <-exp.done:
		case <-time.After(3 * time.Second):
			t.Fatal("no export observed")
		}
		exp.mu.Lock()
		defer exp.mu.Unlock()
		if !exp.ok {
			t.Fatal("export ran with no deadline")
		}
		return time.Until(exp.deadline)
	}

	// Default (unset): the SDK's 30s ceiling.
	if d := budget(t, 0); d > 31*time.Second {
		t.Fatalf("unset export timeout should stay at the 30s default, got ~%s", d)
	}
	// Set above the default: the ceiling is lifted.
	if d := budget(t, 90*time.Second); d < 60*time.Second {
		t.Fatalf("export timeout 90s was capped at ~%s (the 30s default was not lifted)", d)
	}
}
