package mkot

import (
	"maps"

	"gopkg.in/yaml.v3"
)

type ExporterRegistry map[string]ExporterConfigDecoder

var DefaultExporterRegistry = ExporterRegistry{}

func (r ExporterRegistry) Get(name string) (ExporterConfigDecoder, bool) {
	v, ok := r[name]
	return v, ok
}

func (r ExporterRegistry) Set(name string, v ExporterConfigDecoder) {
	r[name] = v
}

func MergeExporterRegistry(a ExporterRegistry, b ExporterRegistry) ExporterRegistry {
	v := maps.Clone(a)
	maps.Copy(v, b)
	return v
}

type ExporterConfigDecoder interface {
	DecodeYamlNode(node *yaml.Node) (ExporterConfig, error)
}

type ExporterConfigDecodable[T ExporterConfig] func() T

func (f ExporterConfigDecodable[T]) DecodeYamlNode(node *yaml.Node) (ExporterConfig, error) {
	c := f()
	if err := node.Decode(c); err != nil {
		return nil, err
	}
	return c, nil
}
