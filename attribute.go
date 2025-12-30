package mkot

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/yaml.v3"
)

type Attribute struct {
	Key   string
	Value attribute.Value
}

func (a Attribute) MarshalYAML() (any, error) {
	return map[string]any{"key": a.Key, "value": a.Value.AsString()}, nil
}

func (a *Attribute) UnmarshalYAML(node *yaml.Node) error {
	type KV struct {
		Key   string
		Value yaml.Node
	}

	var kv KV
	if err := node.Decode(&kv); err != nil {
		return err
	}

	a.Key = kv.Key

	var decoder func(node *yaml.Node) (attribute.Value, error)
	switch kv.Value.Kind {
	case yaml.ScalarNode:
		decoder = a.decodeScalar

	case yaml.SequenceNode:
		decoder = a.decodeSlice
	}

	v, err := decoder(&kv.Value)
	if err != nil {
		return fmt.Errorf("%s: %w", a.Key, err)
	}
	a.Value = v

	return nil
}

func (a *Attribute) decodeScalar(node *yaml.Node) (attribute.Value, error) {
	switch node.Tag {
	case "!!str":
		var v string
		if err := node.Decode(&v); err != nil {
			return attribute.Value{}, err
		}
		return attribute.StringValue(v), nil

	case "!!int":
		var v int
		if err := node.Decode(&v); err != nil {
			return attribute.Value{}, err
		}
		return attribute.IntValue(v), nil

	case "!!float":
		var v float64
		if err := node.Decode(&v); err != nil {
			return attribute.Value{}, err
		}
		return attribute.Float64Value(v), nil

	case "!!bool":
		var v bool
		if err := node.Decode(&v); err != nil {
			return attribute.Value{}, err
		}
		return attribute.BoolValue(v), nil
	}

	return attribute.Value{}, fmt.Errorf("unexpected tag %s", node.Tag)
}

func (a *Attribute) decodeSlice(node *yaml.Node) (attribute.Value, error) {
	vs := []yaml.Node{}
	if err := node.Decode(&vs); err != nil {
		return attribute.Value{}, err
	}
	if len(vs) == 0 {
		return attribute.Int64SliceValue([]int64{}), nil
	}

	n := vs[0]
	switch n.Tag {
	case "!!str":
		v := []string{}
		if err := node.Decode(&v); err != nil {
			return attribute.Value{}, err
		}
		return attribute.StringSliceValue(v), nil

	case "!!int":
		v := []int{}
		if err := node.Decode(&v); err != nil {
			return attribute.Value{}, err
		}
		return attribute.IntSliceValue(v), nil

	case "!!float":
		v := []float64{}
		if err := node.Decode(&v); err != nil {
			return attribute.Value{}, err
		}
		return attribute.Float64SliceValue(v), nil

	case "!!bool":
		v := []bool{}
		if err := node.Decode(&v); err != nil {
			return attribute.Value{}, err
		}
		return attribute.BoolSliceValue(v), nil
	}

	return attribute.Value{}, fmt.Errorf("unexpected tag %s", node.Tag)
}
