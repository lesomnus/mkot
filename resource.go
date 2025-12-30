package mkot

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"gopkg.in/yaml.v3"
)

type ResourceProcessor struct {
	Attributes []Attribute `yaml:",omitempty"`
	Detectors  []string    `yaml:",omitempty"`
}

func (c *ResourceProcessor) TracerOpts(ctx context.Context) ([]trace.TracerProviderOption, error) {
	v, err := resource.New(ctx, c.opts()...)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}
	return []trace.TracerProviderOption{trace.WithResource(v)}, nil
}

func (c *ResourceProcessor) LoggerOpts(ctx context.Context) ([]log.LoggerProviderOption, error) {
	v, err := resource.New(ctx, c.opts()...)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}
	return []log.LoggerProviderOption{log.WithResource(v)}, nil
}

func (c *ResourceProcessor) opts() []resource.Option {
	opts := []resource.Option{}

	kvs := []attribute.KeyValue{}
	for _, a := range c.Attributes {
		kvs = append(kvs, attribute.KeyValue{
			Key:   attribute.Key(a.Key),
			Value: a.Value,
		})
	}
	if len(kvs) > 0 {
		opts = append(opts, resource.WithAttributes(kvs...))
	}

	for _, v := range c.Detectors {
		switch v {
		case "env":
			opts = append(opts, resource.WithFromEnv())

		case "container":
			opts = append(opts, resource.WithContainer())
		case "container.id":
			opts = append(opts, resource.WithContainerID())
		case "host":
			opts = append(opts, resource.WithHost())
		case "host.id":
			opts = append(opts, resource.WithHostID())
		case "os":
			opts = append(opts, resource.WithOS())
		case "os.description":
			opts = append(opts, resource.WithOSDescription())
		case "os.type":
			opts = append(opts, resource.WithOSType())
		case "process":
			opts = append(opts, resource.WithProcess())
		case "process.command_args":
			opts = append(opts, resource.WithProcessCommandArgs())
		case "process.executable.name":
			opts = append(opts, resource.WithProcessExecutableName())
		case "process.executable.path":
			opts = append(opts, resource.WithProcessExecutablePath())
		case "process.owner":
			opts = append(opts, resource.WithProcessOwner())
		case "process.pid":
			opts = append(opts, resource.WithProcessPID())
		case "process.runtime.description":
			opts = append(opts, resource.WithProcessRuntimeDescription())
		case "process.runtime.name":
			opts = append(opts, resource.WithProcessRuntimeName())
		case "process.runtime.vesion":
			opts = append(opts, resource.WithProcessRuntimeVersion())
		case "telemetry.sdk":
			opts = append(opts, resource.WithTelemetrySDK())
		}
	}

	return opts
}

type ResourceProcessorDecoder struct{}

func (d ResourceProcessorDecoder) Decode(node *yaml.Node) (ProcessorConfig, error) {
	var c ResourceProcessor
	if err := node.Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func init() {
	DefaultProcessorRegistry.Set("resource", ResourceProcessorDecoder{})
}
