package otlp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	"github.com/lesomnus/mkot/opaque"
	olog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
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
    timeout: 5s
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
	x.Eq(5*time.Second, e.Timeout)

	// A timeout must build cleanly into every signal's options.
	for _, build := range []func() error{
		func() error { _, err := e.spanOpts(); return err },
		func() error { _, err := e.metricOpts(); return err },
		func() error { _, err := e.logOpts(); return err },
	} {
		x.NoError(build())
	}
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
	t.Run("lowmemory is accepted", func(t *testing.T) {
		_, x := x.New(t)
		_, err := (ExporterConfig{Temporality: "lowmemory"}).metricOpts()
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

// protocol: http must build the OTLP/HTTP exporter and deliver spans as
// protobuf over HTTP to {endpoint}/v1/traces, with http:// implying insecure.
func TestHTTPProtocolConnects(t *testing.T) {
	ctx, x := x.New(t)
	got := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req collectortracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, rs := range req.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, s := range ss.Spans {
					got <- s.Name
				}
			}
		}
		resp, _ := proto.Marshal(&collectortracepb.ExportTraceServiceResponse{})
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(resp)
	}))
	t.Cleanup(srv.Close)

	src := `
exporters:
  otlp:
    protocol: http
    endpoint: "` + srv.URL + `"
providers:
  tracer:
    exporters: [otlp]
`
	var c mkot.Config
	x.NoError(yaml.Unmarshal([]byte(src), &c))
	r := mkot.Make(ctx, &c)
	tp, err := r.Tracer(ctx, "")
	x.NoError(err)
	x.NoError(r.Start(ctx))

	_, span := tp.Tracer("test").Start(ctx, "mkot.http.span")
	span.End()
	x.NoError(r.Shutdown(context.Background()))

	select {
	case name := <-got:
		x.Eq("mkot.http.span", name)
	case <-time.After(3 * time.Second):
		t.Fatal("no export received over OTLP/HTTP")
	}
}

// Every signal must POST to its own /v1/<signal> route. otlploghttp treats a
// path-less URL as an explicit empty path and posted to "/" instead, which a
// collector answers with 404 — losing every log record. A path-bearing endpoint
// is a base URL, so the signal path is appended rather than replacing it.
func TestHTTPEndpointPaths(t *testing.T) {
	for _, tc := range []struct {
		name   string
		suffix string
		want   map[string]string
	}{
		{"scheme only", "", map[string]string{
			"tracer": "/v1/traces", "meter": "/v1/metrics", "logger": "/v1/logs",
		}},
		{"base URL with a prefix", "/otlp", map[string]string{
			"tracer": "/otlp/v1/traces", "meter": "/otlp/v1/metrics", "logger": "/otlp/v1/logs",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for signal, want := range tc.want {
				ctx, x := x.New(t)
				paths := make(chan string, 4)
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					paths <- r.URL.Path
					w.Header().Set("Content-Type", "application/x-protobuf")
					_, _ = w.Write(nil)
				}))
				t.Cleanup(srv.Close)

				src := `
exporters:
  otlp:
    protocol: http/protobuf
    endpoint: "` + srv.URL + tc.suffix + `"
    interval: 100ms
providers:
  ` + signal + `:
    exporters: [otlp]
`
				var c mkot.Config
				x.NoError(yaml.Unmarshal([]byte(src), &c))
				r := mkot.Make(ctx, &c)
				emit, err := emitter(ctx, r, signal)
				x.NoError(err)
				x.NoError(r.Start(ctx))
				emit()
				x.NoError(r.Shutdown(context.Background()))

				select {
				case got := <-paths:
					if got != want {
						t.Fatalf("%s: exported to %q, want %q", signal, got, want)
					}
				case <-time.After(3 * time.Second):
					t.Fatalf("%s: no export received", signal)
				}
			}
		})
	}
}

// emitter wires the provider for one signal and returns a func that records a
// single item through it.
func emitter(ctx context.Context, r mkot.Resolver, signal string) (func(), error) {
	switch signal {
	case "tracer":
		tp, err := r.Tracer(ctx, "")
		if err != nil {
			return nil, err
		}
		return func() {
			_, span := tp.Tracer("test").Start(ctx, "s")
			span.End()
		}, nil
	case "meter":
		mp, err := r.Meter(ctx, "")
		if err != nil {
			return nil, err
		}
		ctr, err := mp.Meter("test").Int64Counter("mkot.test.count")
		if err != nil {
			return nil, err
		}
		return func() { ctr.Add(ctx, 1) }, nil
	default:
		lp, err := r.Logger(ctx, "")
		if err != nil {
			return nil, err
		}
		return func() {
			var rec olog.Record
			rec.SetBody(olog.StringValue("b"))
			lp.Logger("test").Emit(ctx, rec)
		}, nil
	}
}

// gRPC-only knobs must be rejected under protocol http rather than dropped.
func TestHTTPRejectsGRPCOnlyKnobs(t *testing.T) {
	// Every gRPC-only knob, on every HTTP builder.
	for _, e := range []ExporterConfig{
		{Protocol: "http", Authority: "x"},
		{Protocol: "http", BalancerName: "round_robin"},
		{Protocol: "http", ReadBufferSize: 1024},
		{Protocol: "http", WriteBufferSize: 1024},
		{Protocol: "http", WaitForReady: true},
		{Protocol: "http", Keepalive: &KeepaliveConfig{Time: time.Minute}},
		{Protocol: "http", ReconnectionPeriod: time.Second},
	} {
		for name, build := range map[string]func() error{
			"span":   func() error { _, err := e.spanHTTPOpts(); return err },
			"metric": func() error { _, err := e.metricHTTPOpts(); return err },
			"log":    func() error { _, err := e.logHTTPOpts(); return err },
		} {
			if err := build(); err == nil {
				t.Fatalf("%s: expected an error for %+v", name, e)
			}
		}
	}
	// Unknown protocol errors.
	if _, err := (ExporterConfig{Protocol: "thrift"}).protocol(); err == nil {
		t.Fatal("unknown protocol must error")
	}
}

// Every duration knob treats 0 as "use the SDK default", so a negative value
// must be rejected rather than silently taking the same path as unset.
func TestNegativeDurationsRejected(t *testing.T) {
	for _, e := range []ExporterConfig{
		{Timeout: -5 * time.Second},
		{Interval: -time.Second},
		{ReconnectionPeriod: -3 * time.Second},
		{Keepalive: &KeepaliveConfig{Time: -time.Second}},
		{Keepalive: &KeepaliveConfig{Time: time.Minute, Timeout: -time.Second}},
	} {
		for name, build := range map[string]func() error{
			"span":   func() error { _, err := e.spanOpts(); return err },
			"metric": func() error { _, err := e.metricOpts(); return err },
			"log":    func() error { _, err := e.logOpts(); return err },
		} {
			if err := build(); err == nil {
				t.Fatalf("%s: expected an error for %+v", name, e)
			}
		}
	}
	// The HTTP builders share the check (keepalive is rejected there anyway).
	if _, err := (ExporterConfig{Protocol: "http", Timeout: -time.Second}).spanHTTPOpts(); err == nil {
		t.Fatal("http: a negative timeout must error")
	}
}

func TestEndpointScheme(t *testing.T) {
	_, x := x.New(t)
	for _, tc := range []struct {
		ep     string
		scheme bool
	}{
		{"collector:4317", false},
		{"https://collector:4317", true},
		{"http://collector:4317", true},
		// gRPC-native targets are resolved by grpc itself, not WithEndpointURL.
		{"dns:///collector:4317", false},
		{"unix:///var/run/otel.sock", false},
		{"passthrough:///collector:4317", false},
		{"xds:///collector", false},
	} {
		got, err := (ExporterConfig{Endpoint: tc.ep}).endpointHasScheme()
		x.NoError(err)
		x.Eq(tc.scheme, got)
	}
	// A scheme-bearing endpoint must build cleanly (it goes through WithEndpointURL).
	_, err := (ExporterConfig{Endpoint: "https://collector:4317"}).spanOpts()
	x.NoError(err)
}

// A gRPC target must keep building on the gRPC path — rejecting it made a
// sidecar's unix socket inexpressible — while protocol: http, where such a
// scheme is meaningless, still rejects it.
func TestGRPCTargetEndpoints(t *testing.T) {
	_, x := x.New(t)
	for _, ep := range []string{
		"dns:///collector:4317",
		"unix:///var/run/otel.sock",
		"passthrough:///collector:4317",
		"xds:///collector",
	} {
		e := ExporterConfig{Endpoint: ep}
		if _, err := e.spanOpts(); err != nil {
			t.Fatalf("%q: span: %v", ep, err)
		}
		if _, err := e.metricOpts(); err != nil {
			t.Fatalf("%q: metric: %v", ep, err)
		}
		if _, err := e.logOpts(); err != nil {
			t.Fatalf("%q: log: %v", ep, err)
		}

		h := ExporterConfig{Protocol: "http", Endpoint: ep}
		if _, err := h.spanHTTPOpts(); err == nil {
			t.Fatalf("%q: protocol http must reject a gRPC target scheme", ep)
		}
	}
	// A malformed URL is still an error, not a silent pass-through.
	if _, err := (ExporterConfig{Endpoint: "http://[::1"}).spanOpts(); err == nil {
		t.Fatal("malformed endpoint URL must error")
	}
	x.Eq(true, true)
}

// An http:// endpoint scheme must imply insecure and actually connect without an
// explicit tls block; before WithEndpointURL the whole URL was dialed as a
// literal gRPC target and never connected.
func TestSchemeEndpointConnects(t *testing.T) {
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
    endpoint: "http://` + lis.Addr().String() + `"
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

	_, span := tp.Tracer("test").Start(ctx, "mkot.scheme.span")
	span.End()
	err = r.Shutdown(context.Background())
	x.NoError(err)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	x.Eq(true, sink.names["mkot.scheme.span"])
}

func TestExemplarFilterAndReconnection(t *testing.T) {
	_, x := x.New(t)
	for _, v := range []string{"", "always_on", "always_off", "trace_based"} {
		_, err := (ExporterConfig{ExemplarFilter: v}).meterProviderOpts()
		x.NoError(err)
	}
	if _, err := (ExporterConfig{ExemplarFilter: "bogus"}).meterProviderOpts(); err == nil {
		t.Fatal("unknown exemplar_filter must error")
	}
	_, err := (ExporterConfig{ReconnectionPeriod: time.Second}).spanOpts()
	x.NoError(err)
}

// The exemplar_filter must actually reach the MeterProvider: asserting only
// that the option builds cannot tell always_on from always_off. always_on
// records an exemplar for a measurement with no span; always_off records none.
func TestExemplarFilterApplied(t *testing.T) {
	ctx, x := x.New(t)
	for _, tc := range []struct {
		filter string
		want   bool
	}{
		{"always_on", true},
		{"always_off", false},
		{"trace_based", false}, // no sampled span in ctx
	} {
		opts, err := (ExporterConfig{ExemplarFilter: tc.filter}).meterProviderOpts()
		x.NoError(err)
		rdr := metric.NewManualReader()
		mp := metric.NewMeterProvider(append(opts, metric.WithReader(rdr))...)
		ctr, err := mp.Meter("t").Int64Counter("c")
		x.NoError(err)
		ctr.Add(ctx, 5)

		var rm metricdata.ResourceMetrics
		x.NoError(rdr.Collect(ctx, &rm))
		if got := hasExemplar(rm); got != tc.want {
			t.Fatalf("exemplar_filter %q: exemplar present=%v, want %v", tc.filter, got, tc.want)
		}
		x.NoError(mp.Shutdown(ctx))
	}
}

func hasExemplar(rm metricdata.ResourceMetrics) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if s, ok := m.Data.(metricdata.Sum[int64]); ok {
				for _, dp := range s.DataPoints {
					if len(dp.Exemplars) > 0 {
						return true
					}
				}
			}
		}
	}
	return false
}

func TestKeepalivePartialRejected(t *testing.T) {
	_, x := x.New(t)
	// timeout without time is a partial config that grpc would drop whole.
	if _, err := (ExporterConfig{Keepalive: &KeepaliveConfig{Timeout: time.Second}}).spanOpts(); err == nil {
		t.Fatal("keepalive timeout without time must error")
	}
	if _, err := (ExporterConfig{Keepalive: &KeepaliveConfig{PermitWithoutStream: true}}).spanOpts(); err == nil {
		t.Fatal("keepalive permit_without_stream without time must error")
	}
	// A sub-10s time would be silently clamped up to grpc's minimum.
	if _, err := (ExporterConfig{Keepalive: &KeepaliveConfig{Time: time.Second}}).spanOpts(); err == nil {
		t.Fatal("keepalive time below grpc's 10s minimum must error")
	}
	// A full keepalive block, one at exactly the minimum, and an empty one all
	// build cleanly.
	_, err := (ExporterConfig{Keepalive: &KeepaliveConfig{Time: 30 * time.Second, Timeout: time.Second}}).spanOpts()
	x.NoError(err)
	_, err = (ExporterConfig{Keepalive: &KeepaliveConfig{Time: 10 * time.Second}}).spanOpts()
	x.NoError(err)
	_, err = (ExporterConfig{Keepalive: &KeepaliveConfig{}}).spanOpts()
	x.NoError(err)
}

func TestDuplicateHeadersRejected(t *testing.T) {
	_, x := x.New(t)
	e := ExporterConfig{Headers: opaque.MapList{
		{Name: "authorization", Value: "a"},
		{Name: "authorization", Value: "b"},
	}}
	if _, err := e.spanOpts(); err == nil {
		t.Fatal("duplicate header names must error, not silently drop one")
	}
	// Header names are case-insensitive on both transports: net/http would keep
	// one at random and gRPC would send both as one two-valued header.
	e.Headers = opaque.MapList{
		{Name: "Authorization", Value: "a"},
		{Name: "authorization", Value: "b"},
	}
	for _, build := range []func() error{
		func() error { _, err := e.spanOpts(); return err },
		func() error { _, err := e.metricOpts(); return err },
		func() error { _, err := e.logOpts(); return err },
		func() error { _, err := e.spanHTTPOpts(); return err },
	} {
		if err := build(); err == nil {
			t.Fatal("case-differing duplicate header names must error")
		}
	}
	// A distinct set builds cleanly, and is emitted lowercased.
	e.Headers = opaque.MapList{{Name: "Authorization", Value: "a"}, {Name: "x-tenant", Value: "t"}}
	_, err := e.spanOpts()
	x.NoError(err)
	h, err := e.headers()
	x.NoError(err)
	x.Eq("a", h["authorization"])
	x.Eq("t", h["x-tenant"])
}

// The SDK seeds a distinct User-Agent per signal and, unlike the trace exporter,
// the metric/log exporters APPEND mkot's dial options to it. Injecting the trace
// identifier there would overwrite theirs (grpc's WithUserAgent is last-wins).
func TestUserAgentPerSignal(t *testing.T) {
	_, x := x.New(t)
	e := ExporterConfig{ReadBufferSize: 65536}

	tos, err := e.traceDialOpts()
	x.NoError(err)
	x.Eq(2, len(tos)) // the re-seeded UA plus the buffer option

	dos, err := e.dialOpts()
	x.NoError(err)
	x.Eq(1, len(dos)) // metrics/logs get the buffer option only

	// With no dial knob set there is nothing to re-seed.
	none, err := (ExporterConfig{}).traceDialOpts()
	x.NoError(err)
	x.Eq(0, len(none))
}

// An endpoint scheme that contradicts the tls block must not be silently
// resolved: the SDK prioritizes credentials over the scheme's insecure hint, so
// http:// plus a tls block kept TLS on and never reached a plaintext collector.
func TestEndpointTLSConflict(t *testing.T) {
	_, x := x.New(t)
	insecure := &mkot.ClientTlsConfig{}
	insecure.Insecure = true

	for _, tc := range []struct {
		ep  string
		tls *mkot.ClientTlsConfig
	}{
		{"http://collector:4317", &mkot.ClientTlsConfig{}},
		{"https://collector:4317", insecure},
	} {
		e := ExporterConfig{Endpoint: tc.ep, TLS: tc.tls}
		for name, build := range map[string]func() error{
			"span":   func() error { _, err := e.spanOpts(); return err },
			"metric": func() error { _, err := e.metricOpts(); return err },
			"log":    func() error { _, err := e.logOpts(); return err },
		} {
			if err := build(); err == nil {
				t.Fatalf("%s %q with that tls block must error", name, tc.ep)
			}
		}
	}

	// The consistent pairings still build.
	_, err := (ExporterConfig{Endpoint: "http://collector:4317", TLS: insecure}).spanOpts()
	x.NoError(err)
	_, err = (ExporterConfig{Endpoint: "https://collector:4317", TLS: &mkot.ClientTlsConfig{}}).spanOpts()
	x.NoError(err)
	_, err = (ExporterConfig{Endpoint: "http://collector:4317"}).spanOpts()
	x.NoError(err)
}

// uaSink records the gRPC User-Agent of the last export it received.
type uaSink struct {
	collectortracepb.UnimplementedTraceServiceServer
	mu sync.Mutex
	ua string
}

func (s *uaSink) Export(ctx context.Context, _ *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		s.mu.Lock()
		s.ua = strings.Join(md.Get("user-agent"), " ")
		s.mu.Unlock()
	}
	return &collectortracepb.ExportTraceServiceResponse{}, nil
}

// Setting a dial-level knob routes through WithDialOption, which replaces the
// SDK's seeded dial options; the OTel exporter User-Agent identifier must
// survive rather than degrade to a bare grpc-go client.
func TestUserAgentPreservedWithDialOption(t *testing.T) {
	ctx, x := x.New(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	x.NoError(err)
	sink := &uaSink{}
	srv := grpc.NewServer()
	collectortracepb.RegisterTraceServiceServer(srv, sink)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	src := `
exporters:
  otlp:
    endpoint: "` + lis.Addr().String() + `"
    tls: { insecure: true }
    read_buffer_size: 65536
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

	_, span := tp.Tracer("test").Start(ctx, "mkot.ua.span")
	span.End()
	err = r.Shutdown(context.Background())
	x.NoError(err)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	x.Contains(sink.ua, "OTel OTLP Exporter Go")
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

// The configured push interval must reach the reader: with only the
// shutdown-flush asserted, dropping metric.WithInterval left the suite green
// because a 1h interval and the 60s default look identical to it.
func TestMetricIntervalHonored(t *testing.T) {
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
    interval: 100ms
providers:
  meter:
    exporters: [otlp]
`
	var c mkot.Config
	x.NoError(yaml.Unmarshal([]byte(src), &c))
	r := mkot.Make(ctx, &c)
	mp, err := r.Meter(ctx, "")
	x.NoError(err)
	x.NoError(r.Start(ctx))
	t.Cleanup(func() { _ = r.Shutdown(context.Background()) })

	ctr, err := mp.Meter("test").Int64Counter("mkot.interval.count")
	x.NoError(err)
	ctr.Add(ctx, 1)

	// Must arrive from the periodic push alone, with no Shutdown to flush it.
	deadline := time.After(5 * time.Second)
	for !sink.seen("mkot.interval.count") {
		select {
		case <-deadline:
			t.Fatal("no periodic push within 5s: the configured interval was not honored")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// delta and lowmemory differ only on ObservableCounter, and nothing asserted
// which selector was installed — only that building returned no error.
func TestTemporalitySelected(t *testing.T) {
	ctx, x := x.New(t)
	for _, tc := range []struct {
		temporality string
		want        metricdata.Temporality
	}{
		{"", metricdata.CumulativeTemporality},
		{"cumulative", metricdata.CumulativeTemporality},
		{"delta", metricdata.DeltaTemporality},
		{"lowmemory", metricdata.CumulativeTemporality},
	} {
		for _, protocol := range []string{"grpc", "http"} {
			e := ExporterConfig{Protocol: protocol, Temporality: tc.temporality, Endpoint: "127.0.0.1:4317"}
			v, err := e.newMetricExporter(ctx)
			x.NoError(err)
			got := v.Temporality(metric.InstrumentKindObservableCounter)
			if got != tc.want {
				t.Fatalf("%s temporality %q: observable counter is %v, want %v", protocol, tc.temporality, got, tc.want)
			}
			// Counters go delta for both delta and lowmemory.
			if tc.temporality == "delta" || tc.temporality == "lowmemory" {
				if c := v.Temporality(metric.InstrumentKindCounter); c != metricdata.DeltaTemporality {
					t.Fatalf("%s temporality %q: counter is %v, want Delta", protocol, tc.temporality, c)
				}
			}
			x.NoError(v.Shutdown(context.Background()))
		}
	}
}

// service_config and proxy_url are transport-scoped: service_config is gRPC-only
// and conflicts with balancer_name; proxy_url is HTTP-only.
func TestServiceConfigAndProxy(t *testing.T) {
	_, x := x.New(t)

	// service_config builds on gRPC.
	sc := `{"loadBalancingConfig":[{"round_robin":{}}]}`
	_, err := (ExporterConfig{ServiceConfig: sc}).spanOpts()
	x.NoError(err)
	// ...but conflicts with balancer_name.
	if _, err := (ExporterConfig{ServiceConfig: sc, BalancerName: "round_robin"}).spanOpts(); err == nil {
		t.Fatal("service_config + balancer_name must error")
	}
	// ...and is rejected under http.
	if _, err := (ExporterConfig{Protocol: "http", ServiceConfig: sc}).spanHTTPOpts(); err == nil {
		t.Fatal("service_config must be rejected with protocol http")
	}

	// proxy_url builds on http for all three signals.
	for name, build := range map[string]func() error{
		"span": func() error {
			_, err := (ExporterConfig{Protocol: "http", ProxyURL: "http://proxy:3128"}).spanHTTPOpts()
			return err
		},
		"metric": func() error {
			_, err := (ExporterConfig{Protocol: "http", ProxyURL: "http://proxy:3128"}).metricHTTPOpts()
			return err
		},
		"log": func() error {
			_, err := (ExporterConfig{Protocol: "http", ProxyURL: "http://proxy:3128"}).logHTTPOpts()
			return err
		},
	} {
		if err := build(); err != nil {
			t.Fatalf("%s: proxy_url should build on http: %v", name, err)
		}
	}
	// ...is rejected under grpc, and a malformed URL errors.
	if _, err := (ExporterConfig{ProxyURL: "http://proxy:3128"}).spanOpts(); err == nil {
		t.Fatal("proxy_url must be rejected with protocol grpc")
	}
	if _, err := (ExporterConfig{Protocol: "http", ProxyURL: "://bad"}).spanHTTPOpts(); err == nil {
		t.Fatal("a malformed proxy_url must error")
	}
}

// histogram_aggregation selects the default histogram aggregation and reaches
// the exporter for both transports; an unknown value is rejected.
func TestHistogramAggregation(t *testing.T) {
	ctx, x := x.New(t)
	for _, protocol := range []string{"grpc", "http"} {
		e := ExporterConfig{Protocol: protocol, HistogramAggregation: "exponential", Endpoint: "127.0.0.1:4317"}
		v, err := e.newMetricExporter(ctx)
		x.NoError(err)
		agg := v.Aggregation(metric.InstrumentKindHistogram)
		if _, ok := agg.(metric.AggregationBase2ExponentialHistogram); !ok {
			t.Fatalf("%s: histogram aggregation is %T, want base-2 exponential", protocol, agg)
		}
		x.NoError(v.Shutdown(context.Background()))
	}
	// Default stays explicit-bucket.
	v, err := (ExporterConfig{Endpoint: "127.0.0.1:4317"}).newMetricExporter(ctx)
	x.NoError(err)
	if _, ok := v.Aggregation(metric.InstrumentKindHistogram).(metric.AggregationExplicitBucketHistogram); !ok {
		t.Fatal("default histogram aggregation should be explicit-bucket")
	}
	x.NoError(v.Shutdown(context.Background()))
	// Unknown value errors.
	if _, err := (ExporterConfig{HistogramAggregation: "tdigest"}).metricOpts(); err == nil {
		t.Fatal("unknown histogram_aggregation must error")
	}
}
