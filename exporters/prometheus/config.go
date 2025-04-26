package prometheus

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/lesomnus/mkot"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Config struct {
	Endpoint  string
	Namespace string
}

func (c *Config) Tracer(ctx context.Context) (trace.SpanExporter, func(ctx context.Context) error, error) {
	return nil, nil, nil
}

func (c *Config) Meter(ctx context.Context) (metric.Exporter, func(ctx context.Context) error, error) {
	return nil, nil, nil
}

func (c *Config) Reader(ctx context.Context) (metric.Reader, func(ctx context.Context) error, error) {
	reg := prom.NewRegistry()
	opts := []prometheus.Option{prometheus.WithRegisterer(reg)}
	if len(c.Namespace) > 0 {
		opts = append(opts, prometheus.WithNamespace(c.Namespace))
	}

	r, err := prometheus.New(opts...)
	if err != nil {
		return nil, nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	r_ := &reader{
		Exporter: r,

		addr: c.Endpoint,

		reg: reg,
		mux: mux,
	}
	return r_, r_.Start, nil
}

func (c *Config) Logger(ctx context.Context) (log.Exporter, func(ctx context.Context) error, error) {
	return nil, nil, nil
}

type reader struct {
	*prometheus.Exporter
	mu   sync.Mutex
	addr string

	reg *prom.Registry
	mux *http.ServeMux
	srv *http.Server

	started bool
	closed  bool
}

func (r *reader) body() error {
	r.mu.Lock()
	if r.closed {
		return nil
	}

	r.srv = &http.Server{
		Addr:    r.addr,
		Handler: r.mux,
	}
	r.mu.Unlock()

	err := r.srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}

	return err
}

func (r *reader) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return errors.New("already closed")
	}
	if r.started {
		return nil
	}
	r.started = true

	go func() {
		for {
			if err := r.body(); err == nil {
				return
			}

			time.Sleep(5 * time.Second)
		}
	}()

	return nil
}

func (r *reader) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	if r.srv == nil {
		return nil
	}

	return r.srv.Shutdown(ctx)
}

func init() {
	mkot.DefaultExporterRegistry.Set("prometheus", mkot.ExporterConfigDecodeFunc(func() *Config {
		return &Config{}
	}))
}
