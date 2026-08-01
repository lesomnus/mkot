package mkot

import (
	"context"
	"errors"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/lesomnus/mkot/internal/z"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Config struct {
	Enabled *bool `yaml:",omitempty"`

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

func (c Config) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

type ProviderConfig struct {
	Processors []Id `yaml:",omitempty"`
	Exporters  []Id `yaml:",omitempty"`
}

type config struct {
	Enabled *bool

	Processors map[Id]ast.Node
	Exporters  map[Id]ast.Node
	Providers  map[Id]*ProviderConfig
	Service    *serviceConfig
}

// serviceConfig accepts the collector's `service.pipelines` block so a collector
// file's pipeline wiring maps onto mkot providers. Only the pipelines are read;
// `service.extensions` / `service.telemetry` are ignored.
type serviceConfig struct {
	Pipelines map[Id]*pipelineConfig `yaml:"pipelines,omitempty"`
}

type pipelineConfig struct {
	// Receivers are accepted for compatibility but ignored: the application
	// itself is the telemetry source, so mkot has no receivers.
	Receivers  []Id `yaml:"receivers,omitempty"`
	Processors []Id `yaml:"processors,omitempty"`
	Exporters  []Id `yaml:"exporters,omitempty"`
}

// pipelineSignalToProvider maps a collector pipeline signal to the mkot provider
// type. mkot names providers tracer/meter/logger where the collector names
// pipelines traces/metrics/logs.
var pipelineSignalToProvider = map[string]string{
	"traces":  "tracer",
	"metrics": "meter",
	"logs":    "logger",
}

func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	c_ := config{}
	if err := unmarshal(&c_); err != nil {
		return err
	}

	c.Enabled = c_.Enabled

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

	errs_processor := []error{}
	for k, node := range c_.Processors {
		d, ok := reg_processor.New(k.Type())
		if !ok {
			errs_processor = append(errs_processor, fmt.Errorf("%q: unknown type", k.String()))
			continue
		}
		if node != nil {
			raw, err := node.MarshalYAML()
			if err != nil {
				errs_processor = append(errs_processor, fmt.Errorf("%q: get raw: %w", k.String(), err))
			}
			if err := yaml.Unmarshal(raw, d); err != nil {
				errs_processor = append(errs_processor, fmt.Errorf("%q: unmarshal: %w", k.String(), err))
			}
		}

		c.Processors[k] = d
	}

	errs_exporter := []error{}
	for k, node := range c_.Exporters {
		d, ok := reg_exporter.New(k.Type())
		if !ok {
			errs_exporter = append(errs_exporter, fmt.Errorf("%q: unknown type", k.String()))
			continue
		}
		if node != nil {
			raw, err := node.MarshalYAML()
			if err != nil {
				errs_exporter = append(errs_exporter, fmt.Errorf("%q: get raw: %w", k.String(), err))
			}
			if err := yaml.Unmarshal(raw, d); err != nil {
				errs_exporter = append(errs_exporter, fmt.Errorf("%q: unmarshal: %w", k.String(), err))
			}
		}

		c.Exporters[k] = d
	}

	for k, v := range c_.Providers {
		c.Providers[k] = v
	}

	errs_service := []error{}
	if c_.Service != nil {
		for k, p := range c_.Service.Pipelines {
			prov, ok := pipelineSignalToProvider[k.Type()]
			if !ok {
				errs_service = append(errs_service, fmt.Errorf("%q: unknown pipeline signal (want traces, metrics, or logs)", k.String()))
				continue
			}
			id := Id(prov).WithName(k.Name())
			if _, dup := c.Providers[id]; dup {
				errs_service = append(errs_service, fmt.Errorf("%q: also defined under providers.%s", k.String(), id.String()))
				continue
			}
			// Receivers are intentionally ignored (the app is the source).
			c.Providers[id] = &ProviderConfig{Processors: p.Processors, Exporters: p.Exporters}
		}
	}

	return errors.Join(
		z.Err(errors.Join(errs_processor...), ".processor"),
		z.Err(errors.Join(errs_exporter...), ".exporter"),
		z.Err(errors.Join(errs_service...), ".service"),
	)
}

type TracerProviderConfig interface {
	TracerOpts(ctx context.Context) ([]trace.TracerProviderOption, error)
}

type MeterProviderConfig interface {
	MeterOpts(ctx context.Context) ([]metric.Option, error)
}

type LoggerProviderConfig interface {
	LoggerOpts(ctx context.Context) ([]log.LoggerProviderOption, error)
}

type ProcessorConfig interface {
	TracerProviderConfig
	MeterProviderConfig
	LoggerProviderConfig
}

type UnimplementedProcessorConfig struct{}

func (UnimplementedProcessorConfig) TracerOpts(ctx context.Context) ([]trace.TracerProviderOption, error) {
	return nil, nil
}

func (UnimplementedProcessorConfig) MeterOpts(ctx context.Context) ([]metric.Option, error) {
	return nil, nil
}

func (UnimplementedProcessorConfig) LoggerOpts(ctx context.Context) ([]log.LoggerProviderOption, error) {
	return nil, nil
}

type SpanExporterConfig interface {
	SpanExporter(ctx context.Context) (trace.SpanExporter, []trace.TracerProviderOption, error)
}

type MetricExporterConfig interface {
	MetricExporter(ctx context.Context) (metric.Exporter, []metric.Option, error)
}

type MetricReaderConfig interface {
	MetricReader(ctx context.Context) (metric.Reader, []metric.Option, error)
}

type LogExporterConfig interface {
	LogExporter(ctx context.Context) (log.Exporter, []log.LoggerProviderOption, error)
}

type ExporterConfig interface {
	SpanExporterConfig
	MetricExporterConfig
	MetricReaderConfig
	LogExporterConfig
}

type UnimplementedExporterConfig struct{}

func (UnimplementedExporterConfig) SpanExporter(ctx context.Context) (trace.SpanExporter, []trace.TracerProviderOption, error) {
	return nil, nil, ErrUnimplemented
}

func (UnimplementedExporterConfig) MetricExporter(ctx context.Context) (metric.Exporter, []metric.Option, error) {
	return nil, nil, ErrUnimplemented
}

func (UnimplementedExporterConfig) MetricReader(ctx context.Context) (metric.Reader, []metric.Option, error) {
	return nil, nil, ErrUnimplemented
}

func (UnimplementedExporterConfig) LogExporter(ctx context.Context) (log.Exporter, []log.LoggerProviderOption, error) {
	return nil, nil, ErrUnimplemented
}
