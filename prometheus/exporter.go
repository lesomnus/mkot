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
	"go.opentelemetry.io/otel/sdk/metric"
)

var (
	_ mkot.ExporterConfig = (*ExporterConfig)(nil)
	_ metric.Reader       = (*reader)(nil)
)

// ExporterConfig defines common settings for a gRPC client configuration.
type ExporterConfig struct {
	mkot.UnimplementedExporterConfig

	Endpoint  string
	Namespace string
}

func (c *ExporterConfig) MetricReader(ctx context.Context) (metric.Reader, []metric.Option, error) {
	reg := prom.NewRegistry()
	opts := []prometheus.Option{prometheus.WithRegisterer(reg)}
	if c.Namespace != "" {
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
		new_server: func() *http.Server {
			return &http.Server{
				Addr:    c.Endpoint,
				Handler: mux,
			}
		},
		done: make(chan struct{}),
	}
	r_.ctx, r_.cancel = context.WithCancel(context.Background())

	return r_, []metric.Option{metric.WithReader(r_)}, nil
}

type reader struct {
	*prometheus.Exporter

	ctx        context.Context
	cancel     context.CancelFunc
	new_server func() *http.Server

	mu      sync.Mutex
	started bool
	closed  bool

	done chan struct{}
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
		defer close(r.done)
		for {
			s := r.new_server()
			stop := context.AfterFunc(ctx, func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				s.Shutdown(ctx)
			})
			s.ListenAndServe()
			stop()

			select {
			case <-r.ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
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
	r.cancel()
	<-r.done
	return nil
}

func init() {
	mkot.DefaultExporterRegistry.Set("prometheus", func() mkot.ExporterConfig {
		return &ExporterConfig{}
	})
}
