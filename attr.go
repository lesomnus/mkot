package mkot

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"
)

type Attr struct {
	Key   string
	Value attribute.Value
}

func (a Attr) MarshalYAML() (any, error) {
	return map[string]any{"key": a.Key, "value": a.Value.AsString()}, nil
}

func (a *Attr) UnmarshalYAML(unmarshal func(any) error) error {
	type KV struct {
		K string `yaml:"key"`
		V any    `yaml:"value"`
	}

	var kv KV
	if err := unmarshal(&kv); err != nil {
		return err
	}

	v, err := a.decodeValue(kv.V)
	if err != nil {
		return fmt.Errorf("%s: %w", a.Key, err)
	}

	a.Key = kv.K
	a.Value = v

	return nil
}

func (a *Attr) decodeValue(v any) (attribute.Value, error) {
	switch v := v.(type) {
	case string:
		return attribute.StringValue(v), nil
	case uint64:
		return attribute.IntValue(int(v)), nil
	case float64:
		return attribute.Float64Value(v), nil
	case bool:
		return attribute.BoolValue(v), nil
	case []any:
		return a.decodeSlice(v)
	case nil:
		return attribute.Value{}, fmt.Errorf("unexpected null value")
	default:
		return attribute.Value{}, fmt.Errorf("unexpected value type %T", v)
	}
}

func (a *Attr) decodeSlice(vs []any) (attribute.Value, error) {
	if len(vs) == 0 {
		return attribute.Int64SliceValue([]int64{}), nil
	}

	switch vs[0].(type) {
	case string:
		v := []string{}
		for _, item := range vs {
			s, ok := item.(string)
			if !ok {
				return attribute.Value{}, fmt.Errorf("mixed slice types")
			}
			v = append(v, s)
		}
		return attribute.StringSliceValue(v), nil

	case uint64:
		v := []int{}
		for _, item := range vs {
			v = append(v, int(item.(uint64)))
		}
		return attribute.IntSliceValue(v), nil

	case float64:
		v := []float64{}
		for _, item := range vs {
			v = append(v, item.(float64))
		}
		return attribute.Float64SliceValue(v), nil

	case bool:
		v := []bool{}
		for _, item := range vs {
			v = append(v, item.(bool))
		}
		return attribute.BoolSliceValue(v), nil
	}

	return attribute.Value{}, fmt.Errorf("unexpected element type %T", vs[0])
}
