package mkot

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Enabled bool

	ExporterRegistry ExporterRegistry `yaml:",omit"`

	Processors map[Id]ProcessorConfig
	Exporters  map[Id]ExporterConfig
	Providers  map[Id]*ProviderConfig
}

func NewConfig() *Config {
	return &Config{
		ExporterRegistry: DefaultExporterRegistry,

		Processors: map[Id]ProcessorConfig{},
		Exporters:  map[Id]ExporterConfig{},
		Providers:  map[Id]*ProviderConfig{},
	}
}

type ProcessorConfig interface {
	handle(ctx *resolveContext) error
}

// ExporterConfig holds configs to create an exporter
// and provides a method to create the exporter.
// First return value can be nil if the config does not
// support corresponding exporter.
// Second return value is a function that starts the
// exporter and can be nil if the exporter does not
// need to be started or already started.
// Reader will be used when only if Meter is not supported.
type ExporterConfig interface {
	Tracer(ctx context.Context) (trace.SpanExporter, func(ctx context.Context) error, error)
	Meter(ctx context.Context) (metric.Exporter, func(ctx context.Context) error, error)
	Reader(ctx context.Context) (metric.Reader, func(ctx context.Context) error, error)
	Logger(ctx context.Context) (log.Exporter, func(ctx context.Context) error, error)
}

type ProviderConfig struct {
	Processors []Id
	Exporters  []Id
	Pipelines  []string
}

type config struct {
	Enabled bool

	Processors map[Id]yaml.Node
	Exporters  map[Id]yaml.Node
	Providers  map[Id]*ProviderConfig
}

func (c *Config) unmarshalProcessor(k Id, node *yaml.Node) (ProcessorConfig, error) {
	switch k.Type() {
	case "batcher":
		v := &BatcherConfig{}
		if err := node.Decode(v); err != nil {
			return nil, err
		}
		return v, nil

	case "resource":
		v := &ResourceConfig{}
		if err := node.Decode(v); err != nil {
			return nil, err
		}
		return v, nil

	case "periodic_reader":
		v := &PeriodicReaderConfig{}
		if err := node.Decode(v); err != nil {
			return nil, err
		}
		return v, nil

	default:
		return nil, errors.New("unknown type")
	}
}

func (c *Config) unmarshalExporter(k Id, node *yaml.Node) (ExporterConfig, error) {
	r := c.ExporterRegistry
	if r == nil {
		r = DefaultExporterRegistry
	}

	d, ok := r.Get(k.Type())
	if !ok {
		return nil, errors.New("unknown type")
	}

	return d.DecodeYamlNode(node)
}

func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	c_ := config{}
	if err := value.Decode(&c_); err != nil {
		return err
	}

	c.Enabled = c_.Enabled
	if !c.Enabled {
		return nil
	}

	c.Processors = map[Id]ProcessorConfig{}
	c.Exporters = map[Id]ExporterConfig{}
	c.Providers = map[Id]*ProviderConfig{}

	processor_errs := []error{}
	for k, node := range c_.Processors {
		if v, err := c.unmarshalProcessor(k, &node); err != nil {
			processor_errs = append(processor_errs, fmt.Errorf("%q: %w", k.String(), err))
		} else {
			c.Processors[k] = v
		}
	}

	exporter_errs := []error{}
	for k, node := range c_.Exporters {
		if v, err := c.unmarshalExporter(k, &node); err != nil {
			exporter_errs = append(exporter_errs, fmt.Errorf("%q: %w", k.String(), err))
		} else {
			c.Exporters[k] = v
		}
	}

	c.Providers = c_.Providers
	return errors.Join(
		wrapErr("processor", errors.Join(processor_errs...)),
		wrapErr("exporter", errors.Join(exporter_errs...)),
	)
}

func wrapErr(msg string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w", msg, err)
}
