package otlp

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	olog "go.opentelemetry.io/otel/log"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

func TestConfigDecode(t *testing.T) {
	_, x := x.New(t)
	const src = `
exporters:
  otlp:
    endpoint: "collector.local:4317"
    tls: { insecure: true }
    headers:
      - { name: authorization, value: "Bearer x" }
    interval: 30s
    temporality: delta
providers:
  meter:
    exporters: [otlp]
`
	var c mkot.Config
	err := yaml.Unmarshal([]byte(src), &c)
	x.NoError(err)
	e := &ExporterConfig{}
	x.TypeAs(c.Exporters[mkot.Id("otlp")], e)
	x.Eq("collector.local:4317", e.Endpoint)
	x.NotNil(e.TLS)
	x.Eq(true, e.TLS.Insecure)
	auth, ok := e.Headers.Get("authorization")
	x.Eq(true, ok)
	x.Eq("Bearer x", string(auth))
	x.Eq(30*time.Second, e.Interval)
	x.Eq("delta", e.Temporality)
}

func TestRetryPolicy(t *testing.T) {
	t.Run("untouched keeps the exporter defaults", func(t *testing.T) {
		_, x := x.New(t)
		_, ok, err := (ExporterConfig{}).retryPolicy()
		x.NoError(err)
		x.Eq(false, ok)
	})
	t.Run("partial config gets non-zero backoff intervals", func(t *testing.T) {
		_, x := x.New(t)
		p, ok, err := (ExporterConfig{Retry: mkot.RetryConfig{MaxElapsedTime: time.Minute}}).retryPolicy()
		x.NoError(err)
		x.Eq(true, ok)
		x.Eq(true, p.enabled)
		x.Eq(5*time.Second, p.initial)
		x.Eq(30*time.Second, p.max)
		x.Eq(time.Minute, p.elapsed)
	})
	t.Run("explicitly disabled", func(t *testing.T) {
		_, x := x.New(t)
		disabled := false
		p, ok, err := (ExporterConfig{Retry: mkot.RetryConfig{Enabled: &disabled}}).retryPolicy()
		x.NoError(err)
		x.Eq(true, ok)
		x.Eq(false, p.enabled)
	})
	t.Run("unexpressible knobs are rejected, not dropped", func(t *testing.T) {
		e := ExporterConfig{Retry: mkot.RetryConfig{Multiplier: 3}}
		if _, _, err := e.retryPolicy(); err == nil {
			t.Fatal("multiplier is not supported by the exporter and must error")
		}
		if _, err := e.metricOpts(); err == nil {
			t.Fatal("the error must propagate out of the option builders")
		}
	})
}

func TestTemporality(t *testing.T) {
	t.Run("delta selects the SDK delta preference", func(t *testing.T) {
		_, x := x.New(t)
		_, err := (ExporterConfig{Temporality: "delta"}).metricOpts()
		x.NoError(err)
	})
	t.Run("unknown value is rejected", func(t *testing.T) {
		if _, err := (ExporterConfig{Temporality: "bogus"}).metricOpts(); err == nil {
			t.Fatal("unknown temporality must error")
		}
	})
}

func TestCompression(t *testing.T) {
	t.Run("gzip and none are accepted", func(t *testing.T) {
		_, x := x.New(t)
		for _, v := range []string{"", "none", "gzip"} {
			_, err := (ExporterConfig{Compression: v}).spanOpts()
			x.NoError(err)
		}
	})
	t.Run("unsupported values are rejected, not sent uncompressed", func(t *testing.T) {
		for _, v := range []string{"zstd", "snappy", "deflate"} {
			if _, err := (ExporterConfig{Compression: v}).spanOpts(); err == nil {
				t.Fatalf("compression %q must error (grpc only registers gzip)", v)
			}
			if _, err := (ExporterConfig{Compression: v}).metricOpts(); err == nil {
				t.Fatalf("compression %q must error in metricOpts", v)
			}
			if _, err := (ExporterConfig{Compression: v}).logOpts(); err == nil {
				t.Fatalf("compression %q must error in logOpts", v)
			}
		}
	})
}

// metricSink records the metric names pushed to it over OTLP/gRPC.
type metricSink struct {
	collectormetricspb.UnimplementedMetricsServiceServer

	mu    sync.Mutex
	names map[string]bool
}

func (f *metricSink) Export(ctx context.Context, req *collectormetricspb.ExportMetricsServiceRequest) (*collectormetricspb.ExportMetricsServiceResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rm := range req.ResourceMetrics {
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				if f.names == nil {
					f.names = map[string]bool{}
				}
				f.names[m.Name] = true
			}
		}
	}
	return &collectormetricspb.ExportMetricsServiceResponse{}, nil
}

func (f *metricSink) seen(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.names[name]
}

// The reader is the lifecycle component, so the resolver's Shutdown must flush
// the final collection to the collector.
func TestMetricPushFlushedOnShutdown(t *testing.T) {
	ctx, x := x.New(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	x.NoError(err)
	sink := &metricSink{}
	srv := grpc.NewServer()
	collectormetricspb.RegisterMetricsServiceServer(srv, sink)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	src := `
exporters:
  otlp:
    endpoint: "` + lis.Addr().String() + `"
    tls: { insecure: true }
    interval: 1h
providers:
  meter:
    exporters: [otlp]
`
	var c mkot.Config
	err = yaml.Unmarshal([]byte(src), &c)
	x.NoError(err)
	r := mkot.Make(ctx, &c)
	mp, err := r.Meter(ctx, "")
	x.NoError(err)
	err = r.Start(ctx)
	x.NoError(err)

	ctr, err := mp.Meter("test").Int64Counter("mkot.test.count")
	x.NoError(err)
	ctr.Add(ctx, 42)
	err = r.Shutdown(context.Background())
	x.NoError(err)
	x.Eq(true, sink.seen("mkot.test.count"))
}

// traceSink records span names pushed over OTLP/gRPC.
type traceSink struct {
	collectortracepb.UnimplementedTraceServiceServer

	mu    sync.Mutex
	names map[string]bool
}

func (f *traceSink) Export(ctx context.Context, req *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rs := range req.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				if f.names == nil {
					f.names = map[string]bool{}
				}
				f.names[s.Name] = true
			}
		}
	}
	return &collectortracepb.ExportTraceServiceResponse{}, nil
}

// The resolver's Start must be the exporter's single starter (a self-started
// exporter fails a second Start with "already started"), and Shutdown must
// drain the default batch processor so the last spans are not dropped.
func TestSpanPushFlushedOnShutdown(t *testing.T) {
	ctx, x := x.New(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	x.NoError(err)
	sink := &traceSink{}
	srv := grpc.NewServer()
	collectortracepb.RegisterTraceServiceServer(srv, sink)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	src := `
exporters:
  otlp:
    endpoint: "` + lis.Addr().String() + `"
    tls: { insecure: true }
providers:
  tracer:
    exporters: [otlp]
`
	var c mkot.Config
	err = yaml.Unmarshal([]byte(src), &c)
	x.NoError(err)
	r := mkot.Make(ctx, &c)
	tp, err := r.Tracer(ctx, "")
	x.NoError(err)
	err = r.Start(ctx)
	x.NoError(err)

	_, span := tp.Tracer("test").Start(ctx, "mkot.test.span")
	span.End()
	err = r.Shutdown(context.Background())
	x.NoError(err)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	x.Eq(true, sink.names["mkot.test.span"])
}

// logSink records log bodies pushed over OTLP/gRPC.
type logSink struct {
	collectorlogspb.UnimplementedLogsServiceServer

	mu     sync.Mutex
	bodies map[string]bool
}

func (f *logSink) Export(ctx context.Context, req *collectorlogspb.ExportLogsServiceRequest) (*collectorlogspb.ExportLogsServiceResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rl := range req.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			for _, lr := range sl.LogRecords {
				if f.bodies == nil {
					f.bodies = map[string]bool{}
				}
				f.bodies[lr.Body.GetStringValue()] = true
			}
		}
	}
	return &collectorlogspb.ExportLogsServiceResponse{}, nil
}

func TestLogPushFlushedOnShutdown(t *testing.T) {
	ctx, x := x.New(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	x.NoError(err)
	sink := &logSink{}
	srv := grpc.NewServer()
	collectorlogspb.RegisterLogsServiceServer(srv, sink)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	src := `
exporters:
  otlp:
    endpoint: "` + lis.Addr().String() + `"
    tls: { insecure: true }
providers:
  logger:
    exporters: [otlp]
`
	var c mkot.Config
	err = yaml.Unmarshal([]byte(src), &c)
	x.NoError(err)
	r := mkot.Make(ctx, &c)
	lp, err := r.Logger(ctx, "")
	x.NoError(err)
	err = r.Start(ctx)
	x.NoError(err)

	var rec olog.Record
	rec.SetBody(olog.StringValue("mkot.test.log"))
	lp.Logger("test").Emit(ctx, rec)
	err = r.Shutdown(context.Background())
	x.NoError(err)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	x.Eq(true, sink.bodies["mkot.test.log"])
}
