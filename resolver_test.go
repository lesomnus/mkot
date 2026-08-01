package mkot_test

import (
	"context"
	"sync"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	nooplog "go.opentelemetry.io/otel/log/noop"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

// recordingMetricExporter records the metric names handed to it.
type recordingMetricExporter struct {
	metric.Exporter

	mu    sync.Mutex
	names []string
}

func (e *recordingMetricExporter) Temporality(k metric.InstrumentKind) metricdata.Temporality {
	return metric.DefaultTemporalitySelector(k)
}

func (e *recordingMetricExporter) Aggregation(k metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(k)
}

func (e *recordingMetricExporter) Export(_ context.Context, rm *metricdata.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			e.names = append(e.names, m.Name)
		}
	}
	return nil
}

func (e *recordingMetricExporter) ForceFlush(context.Context) error { return nil }
func (e *recordingMetricExporter) Shutdown(context.Context) error   { return nil }

func (e *recordingMetricExporter) exported() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.names...)
}

// bareExporterConfig is a third-party-style exporter that implements only
// MetricExporter, inheriting ErrUnimplemented for MetricReader. No exporter in
// this repo takes that branch, so nothing else covers it.
type bareExporterConfig struct {
	mkot.UnimplementedExporterConfig

	v *recordingMetricExporter
}

func (c *bareExporterConfig) MetricExporter(ctx context.Context) (metric.Exporter, []metric.Option, error) {
	// New contract: return the raw exporter; the resolver installs the reader.
	return c.v, nil, nil
}

// The bare-exporter path holds no reader to collect from, so registering the
// exporter as the shutdown component dropped the final collection entirely.
func TestMeterBareExporterFlushesOnShutdown(t *testing.T) {
	ctx, x := x.New(t)

	v := &recordingMetricExporter{}
	reg := mkot.ExporterRegistry{}
	reg.Set("bare", func() mkot.ExporterConfig { return &bareExporterConfig{v: v} })

	c := mkot.NewConfig()
	c.ExporterRegistry = reg
	x.NoError(yaml.Unmarshal([]byte(`
exporters:
  bare:
providers:
  meter:
    exporters: [bare]
`), c))

	r := mkot.Make(ctx, c)
	mp, err := r.Meter(ctx, "")
	x.NoError(err)
	x.NoError(r.Start(ctx))

	ctr, err := mp.Meter("test").Int64Counter("mkot.bare.count")
	x.NoError(err)
	ctr.Add(ctx, 7)

	x.NoError(r.Shutdown(context.Background()))
	x.Contains(v.exported(), "mkot.bare.count")
}

// Every resolve returns a usable provider alongside its error, so a caller that
// tolerates ErrNotExist - the ordinary case of "this signal is not configured"
// - can use what it was handed without a nil check of its own. Logger used to
// be the one that returned nil.
func TestResolveNotExistYieldsNoopProvider(t *testing.T) {
	ctx, x := x.New(t)

	r := mkot.Make(ctx, nil)

	tracer_provider, err := r.Tracer(ctx, "")
	x.ErrorIs(err, mkot.ErrNotExist)
	x.NotNil(tracer_provider)
	x.Eq(nooptrace.NewTracerProvider(), tracer_provider)

	meter_provider, err := r.Meter(ctx, "")
	x.ErrorIs(err, mkot.ErrNotExist)
	x.NotNil(meter_provider)
	x.Eq(noopmetric.NewMeterProvider(), meter_provider)

	logger_provider, err := r.Logger(ctx, "")
	x.ErrorIs(err, mkot.ErrNotExist)
	x.NotNil(logger_provider)
	x.Eq(nooplog.NewLoggerProvider(), logger_provider)

	// Usable without a nil check: the point of handing one back.
	_, span := tracer_provider.Tracer("test").Start(ctx, "span")
	span.End()
	_, err = meter_provider.Meter("test").Int64Counter("count")
	x.NoError(err)
	logger_provider.Logger("test")
}
