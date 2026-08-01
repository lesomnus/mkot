package mkotx_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	"github.com/lesomnus/mkot/mkotx"
	"github.com/lesomnus/otx"
	nooplog "go.opentelemetry.io/otel/log/noop"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

// recordingSpanExporter records what reached it and refuses anything that
// arrives after it is shut down, which is how a flush that came too late shows
// up.
type recordingSpanExporter struct {
	mu       sync.Mutex
	exported []string
	is_close bool
	is_late  bool
}

func (e *recordingSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.is_close {
		e.is_late = true
		return errors.New("exporter is shut down")
	}
	for _, s := range spans {
		e.exported = append(e.exported, s.Name())
	}

	return nil
}

func (e *recordingSpanExporter) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.is_close = true

	return nil
}

func (e *recordingSpanExporter) Exported() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]string(nil), e.exported...)
}

func (e *recordingSpanExporter) ExportedLate() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.is_late
}

// spanExporterConfig registers the exporter the way the real ones do: unstarted,
// behind a batch processor, with the pair as the lifecycle component.
type spanExporterConfig struct {
	mkot.UnimplementedExporterConfig
	v *recordingSpanExporter
}

func (c *spanExporterConfig) SpanExporter(ctx context.Context) (sdktrace.SpanExporter, []sdktrace.TracerProviderOption, error) {
	p := sdktrace.NewBatchSpanProcessor(c.v)

	return mkot.SpanComponent(c.v, p), []sdktrace.TracerProviderOption{sdktrace.WithSpanProcessor(p)}, nil
}

func configWith(t *testing.T, a x.X, doc string, v *recordingSpanExporter) *mkot.Config {
	t.Helper()

	reg := mkot.ExporterRegistry{}
	reg.Set("rec", func() mkot.ExporterConfig { return &spanExporterConfig{v: v} })

	c := mkot.NewConfig()
	c.ExporterRegistry = reg
	a.NoError(yaml.Unmarshal([]byte(doc), c))

	return c
}

func TestNew(t *testing.T) {
	t.Run("a signal the config does not mention is off rather than an error", func(t *testing.T) {
		ctx, a := x.New(t)

		v, err := mkotx.New(ctx, mkot.Make(ctx, nil), "")
		a.NoError(err)
		a.NotNil(v)

		// Whatever the resolver hands back for a signal it has no config for,
		// which is a no-op provider for all three. The logger is the one worth
		// pinning: it used to be handed back as nil, and this helper would then
		// have left the signal on whatever otx defaults to instead of off.
		a.Eq(nooptrace.NewTracerProvider(), v.Providers().Tracer())
		a.Eq(noopmetric.NewMeterProvider(), v.Providers().Meter())
		a.Eq(nooplog.NewLoggerProvider(), v.Providers().Logger())

		a.NoError(v.Start(ctx))
		a.NoError(v.Shutdown(ctx))
	})
	t.Run("a configured signal reaches its exporter", func(t *testing.T) {
		ctx, a := x.New(t)

		exporter := &recordingSpanExporter{}
		c := configWith(t, a, "exporters:\n  rec:\nproviders:\n  tracer:\n    exporters: [rec]\n", exporter)

		v, err := mkotx.FromConfig(ctx, c, "")
		a.NoError(err)
		a.NoError(v.Start(ctx))

		_, span := v.TraceStart(ctx, "work")
		span.End()
		a.Eq(0, len(exporter.Exported())) // batched: nothing until it is flushed

		a.NoError(v.Shutdown(ctx))
		a.Eq([]string{"work"}, exporter.Exported())
		a.Eq(false, exporter.ExportedLate())
	})
	t.Run("the resolver is the controller", func(t *testing.T) {
		ctx, a := x.New(t)

		r := &recordingResolver{Resolver: mkot.Make(ctx, nil)}
		v, err := mkotx.New(ctx, r, "")
		a.NoError(err)

		a.Eq(0, r.started)
		a.NoError(v.Start(ctx))
		a.Eq(1, r.started)

		a.Eq(0, r.stopped)
		a.NoError(v.Shutdown(ctx))
		a.Eq(1, r.stopped)
	})
	t.Run("an option given by the caller wins", func(t *testing.T) {
		ctx, a := x.New(t)

		tracer_provider := sdktrace.NewTracerProvider()
		v, err := mkotx.New(ctx, mkot.Make(ctx, nil), "",
			otx.WithTracerProvider(tracer_provider),
			otx.WithScopeName("my-service"),
		)
		a.NoError(err)
		a.Eq(tracer_provider, v.Providers().Tracer())
	})
	t.Run("a failure to resolve is returned", func(t *testing.T) {
		ctx, a := x.New(t)

		// An exporter the config names but the registry does not know.
		c := mkot.NewConfig()
		a.NoError(yaml.Unmarshal([]byte("providers:\n  tracer:\n    exporters: [nope]\n"), c))

		// Distinct from a signal that is simply not configured: a config that
		// names an exporter nobody registered is a mistake, not a choice, so
		// it is returned rather than tolerated.
		v, err := mkotx.FromConfig(ctx, c, "")
		if err == nil {
			t.Fatal("want an error")
		}
		if errors.Is(err, mkot.ErrNotExist) {
			t.Fatalf("an unknown exporter was reported as a missing signal: %v", err)
		}
		a.Contains(err.Error(), "nope")
		if v != nil {
			t.Fatalf("an Otx was returned alongside the error: %v", v)
		}
	})
	t.Run("a nil resolver is an error rather than a panic", func(t *testing.T) {
		ctx, a := x.New(t)

		v, err := mkotx.New(ctx, nil, "")
		if err == nil {
			t.Fatal("want an error")
		}
		if v != nil {
			t.Fatal("want no Otx")
		}
		a.Eq(true, len(err.Error()) > 0)
	})
}

// recordingResolver counts the lifecycle calls the Otx makes on it.
type recordingResolver struct {
	mkot.Resolver

	started int
	stopped int
}

func (r *recordingResolver) Start(ctx context.Context) error {
	r.started++

	return r.Resolver.Start(ctx)
}

func (r *recordingResolver) Shutdown(ctx context.Context) error {
	r.stopped++

	return r.Resolver.Shutdown(ctx)
}
