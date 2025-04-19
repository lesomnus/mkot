package mkot

import (
	"context"
	"fmt"

	olog "go.opentelemetry.io/otel/log"
	ometric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	otrace "go.opentelemetry.io/otel/trace"
)

type Resolver interface {
	Tracer(ctx context.Context, name string, opts ...trace.TracerProviderOption) (otrace.TracerProvider, error)
	Meter(ctx context.Context, name string, opts ...metric.Option) (ometric.MeterProvider, error)
	Logger(ctx context.Context, name string, opts ...log.LoggerProviderOption) (olog.LoggerProvider, error)
}

func Make(ctx context.Context, c *Config) Resolver {
	return &resolver{
		config:    c,
		exporters: map[Id]*exporterSet{},
	}
}

type resolver struct {
	config    *Config
	exporters map[Id]*exporterSet
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
			opts: []trace.TracerProviderOption{},
		},
		metric: meterResolverContext{
			opts: []metric.Option{},
		},
		log: loggerResolverContext{
			opts: []log.LoggerProviderOption{},
		},
	}
}

func (r *resolver) resolveExporter(ctx context.Context, id Id) (*exporterSet, error) {
	if v, ok := r.exporters[id]; ok {
		return v, nil
	}

	c, ok := r.config.Exporters[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	v, err := newExporterSet(ctx, c)
	if err != nil {
		return nil, err
	}

	r.exporters[id] = v
	return v, nil
}

func (r *resolver) Tracer(ctx context.Context, name string, opts ...trace.TracerProviderOption) (otrace.TracerProvider, error) {
	c, ok := r.config.Providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q: not found", name)
	}

	ctx_ := newResolveContext(ctx)
	ctx_.trace.opts = append(ctx_.trace.opts, opts...)
	for _, id := range c.Exporters {
		v, err := r.resolveExporter(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("exporter %q: %w", id, err)
		}

		ctx_.trace.exporters = append(ctx_.trace.exporters, v.tracer)
	}
	for _, id := range c.Processors {
		processor_config, ok := r.config.Processors[id]
		if !ok {
			return nil, fmt.Errorf("processor %q: not found", id.String())
		}

		processor_config.handle(ctx_)
	}
	if !ctx_.trace.is_touched {
		return nil, fmt.Errorf("no span processor is specified")
	}

	v := trace.NewTracerProvider(ctx_.trace.opts...)
	return v, nil
}

func (r *resolver) Meter(ctx context.Context, name string, opts ...metric.Option) (ometric.MeterProvider, error) {
	c, ok := r.config.Providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q: not found", name)
	}

	ctx_ := newResolveContext(ctx)
	ctx_.metric.opts = append(ctx_.metric.opts, opts...)
	for _, id := range c.Exporters {
		v, err := r.resolveExporter(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("exporter %q: %w", id, err)
		}

		ctx_.metric.exporters = append(ctx_.metric.exporters, v.meter)
	}
	for _, id := range c.Processors {
		processor_config, ok := r.config.Processors[id]
		if !ok {
			return nil, fmt.Errorf("processor %q: not found", id.String())
		}

		processor_config.handle(ctx_)
	}
	if !ctx_.trace.is_touched {
		return nil, fmt.Errorf("no span processor is specified")
	}

	v := metric.NewMeterProvider(ctx_.metric.opts...)
	return v, nil
}

func (r *resolver) Logger(ctx context.Context, name string, opts ...log.LoggerProviderOption) (olog.LoggerProvider, error) {
	c, ok := r.config.Providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q: not found", name)
	}

	ctx_ := newResolveContext(ctx)
	ctx_.log.opts = append(ctx_.log.opts, opts...)
	for _, id := range c.Exporters {
		v, err := r.resolveExporter(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("exporter %q: %w", id, err)
		}

		ctx_.log.exporters = append(ctx_.log.exporters, v.logger)
	}
	for _, id := range c.Processors {
		processor_config, ok := r.config.Processors[id]
		if !ok {
			return nil, fmt.Errorf("processor %q: not found", id.String())
		}

		processor_config.handle(ctx_)
	}
	if !ctx_.trace.is_touched {
		return nil, fmt.Errorf("no span processor is specified")
	}

	v := log.NewLoggerProvider(ctx_.log.opts...)
	return v, nil
}

type exporterSet struct {
	tracer trace.SpanExporter
	meter  metric.Exporter
	logger log.Exporter
}

func newExporterSet(ctx context.Context, c ExporterConfig) (*exporterSet, error) {
	tracer, err := c.tracer(ctx)
	if err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}
	meter, err := c.meter(ctx)
	if err != nil {
		return nil, fmt.Errorf("meter: %w", err)
	}
	logger, err := c.logger(ctx)
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	return &exporterSet{
		tracer: tracer,
		meter:  meter,
		logger: logger,
	}, nil
}
