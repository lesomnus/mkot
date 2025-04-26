package mkot

import (
	"context"
	"errors"
	"fmt"

	olog "go.opentelemetry.io/otel/log"
	ometric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	otrace "go.opentelemetry.io/otel/trace"
)

// Resolver constructs providers from the config it is based on.
// The providers are assumed to be unstarted before [Resolver.Start] is called.
type Resolver interface {
	Tracer(ctx context.Context, name string, opts ...trace.TracerProviderOption) (otrace.TracerProvider, error)
	Meter(ctx context.Context, name string, opts ...metric.Option) (ometric.MeterProvider, error)
	Logger(ctx context.Context, name string, opts ...log.LoggerProviderOption) (olog.LoggerProvider, error)

	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

func Make(ctx context.Context, c *Config) Resolver {
	if c == nil {
		c = NewConfig()
	}
	if c.Processors == nil {
		c.Processors = map[Id]ProcessorConfig{}
	}
	if c.Exporters == nil {
		c.Exporters = map[Id]ExporterConfig{}
	}
	if c.Providers == nil {
		c.Providers = map[Id]*ProviderConfig{}
	}
	return &resolver{
		config: c,

		span_exporters:   map[Id]exporter[trace.SpanExporter]{},
		metric_exporters: map[Id]exporter[metric.Exporter]{},
		metric_readers:   map[Id]exporter[metric.Reader]{},
		log_exporters:    map[Id]exporter[log.Exporter]{},
	}
}

type exporter[T any] struct {
	value T
	start func(ctx context.Context) error
}

func (e exporter[T]) Start(ctx context.Context) error {
	if e.start == nil {
		return nil
	}

	return e.start(ctx)
}

type resolver struct {
	config *Config

	span_exporters   map[Id]exporter[trace.SpanExporter]
	metric_exporters map[Id]exporter[metric.Exporter]
	metric_readers   map[Id]exporter[metric.Reader]
	log_exporters    map[Id]exporter[log.Exporter]
}

func (r *resolver) Start(ctx context.Context) error {
	var err error
	check := func(
		id Id,
		f interface {
			Start(ctx context.Context) error
		},
	) {
		if f == nil {
			return
		}

		err = f.Start(ctx)
		if err != nil {
			err = fmt.Errorf("%q: %w", id, err)
		}
	}

	for id, v := range r.span_exporters {
		if err != nil {
			break
		}
		check(id, v)
	}
	for id, v := range r.metric_exporters {
		if err != nil {
			break
		}
		check(id, v)
	}
	for id, v := range r.metric_readers {
		if err != nil {
			break
		}
		check(id, v)
	}
	for id, v := range r.log_exporters {
		if err != nil {
			break
		}
		check(id, v)
	}
	if err == nil {
		return nil
	}

	err_ := r.Shutdown(ctx)
	if err_ != nil {
		err_ = fmt.Errorf("shutdown: %w", err_)
	}

	return errors.Join(err, err_)
}

func (r *resolver) Shutdown(ctx context.Context) error {
	errs := []error{}
	check := func(
		id Id,
		f interface {
			Shutdown(ctx context.Context) error
		},
	) {
		if f == nil {
			return
		}

		err := f.Shutdown(ctx)
		if err != nil {
			err = fmt.Errorf("%q: %w", id, err)
			errs = append(errs, err)
		}
	}

	for id, v := range r.span_exporters {
		check(id, v.value)
	}
	for id, v := range r.metric_exporters {
		check(id, v.value)
	}
	for id, v := range r.metric_readers {
		check(id, v.value)
	}
	for id, v := range r.log_exporters {
		check(id, v.value)
	}

	return errors.Join(errs...)
}

type resolveContext struct {
	context.Context

	resource *resource.Resource

	trace  tracerResolverContext
	metric meterResolverContext
	log    loggerResolverContext
}

type tracerResolverContext struct {
	opts []trace.TracerProviderOption

	exporters  []trace.SpanExporter
	is_touched bool
}

type meterResolverContext struct {
	opts []metric.Option

	exporters []metric.Exporter
	readers   []metric.Reader
}

type loggerResolverContext struct {
	opts []log.LoggerProviderOption

	exporters  []log.Exporter
	is_touched bool
}

func newResolveContext(ctx context.Context) *resolveContext {
	return &resolveContext{
		Context: ctx,
		trace: tracerResolverContext{
			opts:      []trace.TracerProviderOption{},
			exporters: []trace.SpanExporter{},
		},
		metric: meterResolverContext{
			opts:      []metric.Option{},
			exporters: []metric.Exporter{},
			readers:   []metric.Reader{},
		},
		log: loggerResolverContext{
			opts:      []log.LoggerProviderOption{},
			exporters: []log.Exporter{},
		},
	}
}

func (r *resolver) Tracer(ctx context.Context, name string, opts ...trace.TracerProviderOption) (otrace.TracerProvider, error) {
	id := Id("tracer").WithName(name)
	c, ok := r.config.Providers[id]
	if !ok {
		return nil, fmt.Errorf("provider %q: not found", id)
	}

	ctx_ := newResolveContext(ctx)
	ctx_.trace.opts = append(ctx_.trace.opts, opts...)
	for _, id := range c.Exporters {
		v, err := r.getSpanExporter(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("exporter %q: %w", id, err)
		}
		if v.value == nil {
			continue
		}

		ctx_.trace.exporters = append(ctx_.trace.exporters, v.value)
	}
	for _, id := range c.Processors {
		processor_config, ok := r.config.Processors[id]
		if !ok {
			return nil, fmt.Errorf("processor %q: not found", id.String())
		}

		processor_config.handle(ctx_)
	}
	if !ctx_.trace.is_touched {
		for _, e := range ctx_.trace.exporters {
			ctx_.trace.opts = append(ctx_.trace.opts, trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(e)))
		}
	}

	ctx_.trace.opts = append(ctx_.trace.opts, trace.WithResource(ctx_.resource))
	v := trace.NewTracerProvider(ctx_.trace.opts...)
	return v, nil
}

func (r *resolver) getSpanExporter(ctx context.Context, id Id) (exporter[trace.SpanExporter], error) {
	v, ok := r.span_exporters[id]
	if ok {
		return v, nil
	}

	f, ok := r.config.Exporters[id]
	if !ok {
		return v, errors.New("not found")
	}

	exporter, start, err := f.Tracer(ctx)
	if err != nil {
		return v, fmt.Errorf("create span exporter: %w", err)
	}

	v.value = exporter
	v.start = start
	r.span_exporters[id] = v

	return v, nil
}

func (r *resolver) Meter(ctx context.Context, name string, opts ...metric.Option) (ometric.MeterProvider, error) {
	id := Id("meter").WithName(name)
	c, ok := r.config.Providers[id]
	if !ok {
		return nil, fmt.Errorf("provider %q: not found", id)
	}

	ctx_ := newResolveContext(ctx)
	ctx_.metric.opts = append(ctx_.metric.opts, opts...)
	for _, id := range c.Exporters {
		if v, err := r.getMetricExporter(ctx, id); err != nil {
			return nil, fmt.Errorf("exporter %q: %w", id, err)
		} else if v.value != nil {
			ctx_.metric.exporters = append(ctx_.metric.exporters, v.value)
			continue
		}

		if v, err := r.getMetricReader(ctx, id); err != nil {
			return nil, fmt.Errorf("exporter %q: %w", id, err)
		} else if v.value != nil {
			ctx_.metric.readers = append(ctx_.metric.readers, v.value)
		}
	}
	for _, id := range c.Processors {
		processor_config, ok := r.config.Processors[id]
		if !ok {
			return nil, fmt.Errorf("processor %q: not found", id.String())
		}

		processor_config.handle(ctx_)
	}
	for _, r := range ctx_.metric.readers {
		ctx_.metric.opts = append(ctx_.metric.opts, metric.WithReader(r))
	}

	ctx_.metric.opts = append(ctx_.metric.opts, metric.WithResource(ctx_.resource))
	v := metric.NewMeterProvider(ctx_.metric.opts...)
	return v, nil
}

func (r *resolver) getMetricExporter(ctx context.Context, id Id) (exporter[metric.Exporter], error) {
	v, ok := r.metric_exporters[id]
	if ok {
		return v, nil
	}

	f, ok := r.config.Exporters[id]
	if !ok {
		return v, errors.New("not found")
	}

	w, start, err := f.Meter(ctx)
	if err != nil {
		return v, fmt.Errorf("create metric exporter: %w", err)
	}

	v.value = w
	v.start = start
	r.metric_exporters[id] = v

	return v, nil
}

func (r *resolver) getMetricReader(ctx context.Context, id Id) (exporter[metric.Reader], error) {
	v, ok := r.metric_readers[id]
	if ok {
		return v, nil
	}

	f, ok := r.config.Exporters[id]
	if !ok {
		return v, errors.New("not found")
	}

	w, start, err := f.Reader(ctx)
	if err != nil {
		return v, fmt.Errorf("create metric reader: %w", err)
	}

	v.value = w
	v.start = start
	r.metric_readers[id] = v

	return v, nil
}

func (r *resolver) Logger(ctx context.Context, name string, opts ...log.LoggerProviderOption) (olog.LoggerProvider, error) {
	id := Id("logger").WithName(name)
	c, ok := r.config.Providers[id]
	if !ok {
		return nil, fmt.Errorf("provider %q: not found", id)
	}

	ctx_ := newResolveContext(ctx)
	ctx_.log.opts = append(ctx_.log.opts, opts...)
	for _, id := range c.Exporters {
		v, err := r.getLogExporter(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("exporter %q: %w", id, err)
		}
		if v.value == nil {
			continue
		}

		ctx_.log.exporters = append(ctx_.log.exporters, v.value)
	}
	for _, id := range c.Processors {
		processor_config, ok := r.config.Processors[id]
		if !ok {
			return nil, fmt.Errorf("processor %q: not found", id.String())
		}

		processor_config.handle(ctx_)
	}
	if !ctx_.log.is_touched {
		for _, e := range ctx_.log.exporters {
			ctx_.log.opts = append(ctx_.log.opts, log.WithProcessor(log.NewSimpleProcessor(e)))
		}
	}

	ctx_.log.opts = append(ctx_.log.opts, log.WithResource(ctx_.resource))
	v := log.NewLoggerProvider(ctx_.log.opts...)
	return v, nil
}

func (r *resolver) getLogExporter(ctx context.Context, id Id) (exporter[log.Exporter], error) {
	v, ok := r.log_exporters[id]
	if ok {
		return v, nil
	}

	f, ok := r.config.Exporters[id]
	if !ok {
		return v, errors.New("not found")
	}

	w, start, err := f.Logger(ctx)
	if err != nil {
		return v, fmt.Errorf("create log exporter: %w", err)
	}

	v.value = w
	v.start = start
	r.log_exporters[id] = v

	return v, nil
}
