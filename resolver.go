package mkot

import (
	"context"
	"errors"
	"fmt"

	olog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/trace"
	otrace "go.opentelemetry.io/otel/trace"
)

// Resolver constructs providers from the config it is based on.
// The providers are assumed to be unstarted before [Resolver.Start] is called.
type Resolver interface {
	Tracer(ctx context.Context, name string, opts ...trace.TracerProviderOption) (otrace.TracerProvider, error)
	// Meter(ctx context.Context, name string, opts ...metric.Option) (ometric.MeterProvider, error)
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
		config:    c,
		providers: map[Id]*provider{},
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
	config    *Config
	providers map[Id]*provider
}

type provider struct {
	value      any
	components map[Id]any
}

func (r *resolver) Start(ctx context.Context) (err error) {
	components := []any{}
	defer func() {
		if err == nil {
			return
		}

		for _, c := range components {
			c_, ok := c.(interface {
				Shutdown(ctx context.Context) error
			})
			if !ok {
				continue
			}

			c_.Shutdown(ctx)
		}
	}()

	for pid, p := range r.providers {
		for cid, c := range p.components {
			c_, ok := c.(interface {
				Start(ctx context.Context) error
			})
			if !ok {
				continue
			}

			if err = c_.Start(ctx); err != nil {
				return fmt.Errorf("provider[%q].component[%q]: %w", pid, cid, err)
			}

			components = append(components, c_)
		}
	}

	return nil
}

func (r *resolver) Shutdown(ctx context.Context) error {
	errs := []error{}
	for pid, p := range r.providers {
		for cid, c := range p.components {
			c_, ok := c.(interface {
				Shutdown(ctx context.Context) error
			})
			if !ok {
				continue
			}

			err := c_.Shutdown(ctx)
			if err != nil {
				errs = append(errs, fmt.Errorf("provider[%q][%q]: %w", pid, cid, err))
			}
		}
	}

	return errors.Join(errs...)
}

func (r *resolver) Tracer(ctx context.Context, name string, opts ...trace.TracerProviderOption) (otrace.TracerProvider, error) {
	id := Id("tracer").WithName(name)
	if r.providers == nil {
		r.providers = map[Id]*provider{}
	}
	if p, ok := r.providers[id]; ok {
		return p.value.(otrace.TracerProvider), nil
	}

	c, ok := r.config.Providers[id]
	if !ok {
		return nil, fmt.Errorf("undefined provider %q", id)
	}

	components := map[Id]any{}
	for _, id := range c.Processors {
		if err := func() error {
			c, ok := r.config.Processors[id]
			if !ok {
				return fmt.Errorf("not found")
			}

			c_, ok := c.(TracerProviderConfig)
			if !ok {
				return fmt.Errorf("not for the tracer")
			}

			opts_, err := c_.TracerOpts(ctx)
			if err != nil {
				return err
			}
			if opts_ == nil {
				return fmt.Errorf("not for the tracer")
			}

			opts = append(opts, opts_...)
			return nil
		}(); err != nil {
			return nil, fmt.Errorf("processor %q: %w", id.String(), err)
		}
	}
	for _, id := range c.Exporters {
		if err := func() error {
			c, ok := r.config.Exporters[id]
			if !ok {
				return fmt.Errorf("not found")
			}

			c_, ok := c.(SpanExporterConfig)
			if !ok {
				return fmt.Errorf("not a span exporter")
			}

			v, opts_, err := c_.SpanExporter(ctx)
			if err != nil {
				return err
			}

			components[id] = v
			opts = append(opts, opts_...)
			return nil
		}(); err != nil {
			return nil, fmt.Errorf("exporter %q: %w", id.String(), err)
		}

	}

	v := trace.NewTracerProvider(opts...)
	r.providers[id] = &provider{
		value:      v,
		components: components,
	}
	return v, nil
}

func (r *resolver) Logger(ctx context.Context, name string, opts ...log.LoggerProviderOption) (olog.LoggerProvider, error) {
	id := Id("logger").WithName(name)
	if r.providers == nil {
		r.providers = map[Id]*provider{}
	}
	if p, ok := r.providers[id]; ok {
		return p.value.(olog.LoggerProvider), nil
	}

	c, ok := r.config.Providers[id]
	if !ok {
		return nil, fmt.Errorf("undefined provider %q", id)
	}

	components := map[Id]any{}
	for _, id := range c.Processors {
		if err := func() error {
			c, ok := r.config.Processors[id]
			if !ok {
				return fmt.Errorf("not found")
			}

			c_, ok := c.(LoggerProviderConfig)
			if !ok {
				return fmt.Errorf("not for the logger")
			}

			opts_, err := c_.LoggerOpts(ctx)
			if err != nil {
				return err
			}
			if opts_ == nil {
				return fmt.Errorf("not for the logger")
			}

			opts = append(opts, opts_...)
			return nil
		}(); err != nil {
			return nil, fmt.Errorf("processor %q: %w", id.String(), err)
		}
	}
	for _, id := range c.Exporters {
		if err := func() error {
			c, ok := r.config.Exporters[id]
			if !ok {
				return fmt.Errorf("not found")
			}

			c_, ok := c.(LogExporterConfig)
			if !ok {
				return fmt.Errorf("not a log exporter")

			}

			v, opts_, err := c_.LogExporter(ctx)
			if err != nil {
				return err
			}

			components[id] = v
			opts = append(opts, opts_...)
			return nil
		}(); err != nil {
			return nil, fmt.Errorf("exporter %q: %w", id.String(), err)
		}
	}

	v := log.NewLoggerProvider(opts...)
	r.providers[id] = &provider{
		value:      v,
		components: components,
	}
	return v, nil
}
