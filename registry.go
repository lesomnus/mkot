package mkot

import (
	"maps"
)

var (
	DefaultProcessorRegistry = ProcessorRegistry{}
	DefaultExporterRegistry  = ExporterRegistry{}
)

type Registry[T any] map[string]ConfigDecoder[T]

func (r Registry[T]) Get(name string) (ConfigDecoder[T], bool) {
	v, ok := r[name]
	return v, ok
}

func (r Registry[T]) Set(name string, v ConfigDecoder[T]) {
	r[name] = v
}

func MergeRegistry[T any](a Registry[T], b Registry[T]) Registry[T] {
	v := Registry[T]{}
	if a == nil && b == nil {
		return v
	}
	if a != nil {
		maps.Copy(v, a)
	}
	if b != nil {
		maps.Copy(v, b)
	}

	return v
}

type ProcessorRegistry = Registry[ProcessorConfig]
type ExporterRegistry = Registry[ExporterConfig]
