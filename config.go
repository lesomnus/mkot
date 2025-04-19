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

	Processors map[Id]ProcessorConfig
	Exporters  map[Id]ExporterConfig
	Providers  map[string]ProviderConfig
}

type ProcessorConfig interface {
	handle(ctx *resolveContext) error
}

type ExporterConfig interface {
	tracer(ctx context.Context) (trace.SpanExporter, error)
	meter(ctx context.Context) (metric.Exporter, error)
	logger(ctx context.Context) (log.Exporter, error)
}

type ProviderConfig struct {
	Processors []Id
	Exporters  []Id
}

type config struct {
	Enabled bool

	Processors map[Id]yaml.Node
	Exporters  map[Id]yaml.Node
	Providers  map[string]ProviderConfig
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
	switch k.Type() {
	case "debug":
		v := &DebugExporterConfig{}
		if err := node.Decode(v); err != nil {
			return nil, err
		}
		return v, nil

	case "otlp":
		v := &OtlpExporterConfig{}
		if err := node.Decode(v); err != nil {
			return nil, err
		}
		return v, nil

	default:
		return nil, errors.New("unknown type")
	}
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
	c.Providers = map[string]ProviderConfig{}

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
