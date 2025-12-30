package pretty

import (
	"io"
	"os"

	"github.com/lesomnus/mkot"
	"gopkg.in/yaml.v3"
)

type Config struct {
	filename string
	Output   io.WriteCloser
}

func (c Config) MarshalYAML() (any, error) {
	if c.filename == "" {
		switch c.Output {
		case os.Stdout:
			c.filename = "&stdout"
		case os.Stderr:
			c.filename = "&stderr"
		default:
			return nil, nil
		}
	}
	return map[string]any{"output": c.filename}, nil
}

func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	type T struct {
		Output string
	}

	var t T
	if err := node.Decode(&t); err != nil {
		return err
	}

	c.filename = t.Output
	switch t.Output {
	case "", "&stdout":
		c.filename = "&stdout"
		c.Output = os.Stdout
	case "&stderr":
		c.Output = os.Stderr
	default:
		f, err := os.OpenFile(t.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		c.Output = f
	}

	return nil
}

func decode(node *yaml.Node) (*Config, error) {
	var v Config
	if err := node.Decode(&v); err != nil {
		return nil, err
	}

	v.Output = os.Stdout
	return &v, nil
}

type processorDecoder struct{}

func (processorDecoder) Decode(node *yaml.Node) (mkot.ProcessorConfig, error) {
	return decode(node)
}

type exporterDecoder struct{}

func (exporterDecoder) Decode(node *yaml.Node) (mkot.ExporterConfig, error) {
	return decode(node)
}

func init() {
	mkot.DefaultProcessorRegistry.Set("pretty", processorDecoder{})
	mkot.DefaultExporterRegistry.Set("pretty", exporterDecoder{})
}
