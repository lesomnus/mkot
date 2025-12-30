package mkot

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/trace"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Enabled bool

	ProcessorRegistry ProcessorRegistry `yaml:"-"`
	ExporterRegistry  ExporterRegistry  `yaml:"-"`

	Processors map[Id]ProcessorConfig `yaml:",omitempty"`
	Exporters  map[Id]ExporterConfig  `yaml:",omitempty"`
	Providers  map[Id]*ProviderConfig `yaml:",omitempty"`
}

func NewConfig() *Config {
	return &Config{
		ProcessorRegistry: DefaultProcessorRegistry,
		ExporterRegistry:  DefaultExporterRegistry,

		Processors: map[Id]ProcessorConfig{},
		Exporters:  map[Id]ExporterConfig{},
		Providers:  map[Id]*ProviderConfig{},
	}
}

type TracerOpts interface {
	TracerOpts(ctx context.Context) ([]trace.TracerProviderOption, error)
}

type LoggerOpts interface {
	LoggerOpts(ctx context.Context) ([]log.LoggerProviderOption, error)
}

type ProcessorConfig interface{}

type SpanExporter interface {
	SpanExporter(ctx context.Context) (trace.SpanExporter, error)
}

type LogExporter interface {
	LogExporter(ctx context.Context) (log.Exporter, error)
}

type ExporterConfig interface{}

type ProviderConfig struct {
	Processors []Id `yaml:",omitempty"`
	Exporters  []Id `yaml:",omitempty"`
}

type config struct {
	Enabled bool

	Processors map[Id]yaml.Node
	Exporters  map[Id]yaml.Node
	Providers  map[Id]*ProviderConfig
}

func (c Config) MarshalYAML() (any, error) {
	type T Config
	return T(c), nil
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

	reg_processor := c.ProcessorRegistry
	if reg_processor == nil {
		reg_processor = DefaultProcessorRegistry
	}

	reg_exporter := c.ExporterRegistry
	if reg_exporter == nil {
		reg_exporter = DefaultExporterRegistry
	}

	c.Processors = map[Id]ProcessorConfig{}
	c.Exporters = map[Id]ExporterConfig{}
	c.Providers = map[Id]*ProviderConfig{}

	processor_errs := []error{}
	for k, node := range c_.Processors {
		d, ok := reg_processor.Get(k.Type())
		if !ok {
			processor_errs = append(processor_errs, fmt.Errorf("%q: unknown type", k.String()))
			continue
		}

		c_, err := d.Decode(&node)
		if err != nil {
			processor_errs = append(processor_errs, fmt.Errorf("%q: %w", k.String(), err))
			continue
		}

		c.Processors[k] = c_
	}

	exporter_errs := []error{}
	for k, node := range c_.Exporters {
		d, ok := reg_exporter.Get(k.Type())
		if !ok {
			exporter_errs = append(exporter_errs, fmt.Errorf("%q: unknown type", k.String()))
			continue
		}

		c_, err := d.Decode(&node)
		if err != nil {
			exporter_errs = append(exporter_errs, fmt.Errorf("%q: %w", k.String(), err))
			continue
		}

		c.Exporters[k] = c_
	}

	c.Providers = c_.Providers
	return errors.Join(
		wrapErr("processor", errors.Join(processor_errs...)),
		wrapErr("exporter", errors.Join(exporter_errs...)),
	)
}

type ConfigDecoder[T any] interface {
	Decode(node *yaml.Node) (T, error)
}

func wrapErr(msg string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w", msg, err)
}
