package pretty

import (
	"io"

	"go.opentelemetry.io/otel/attribute"
)

func renderAttrValue(o io.Writer, v attribute.Value) error {
	const (
		maxBytes = 8
		maxSlice = 5
		maxMap   = 5
	)

	c := attr_colors[v.Type()]
	switch v.Type() {
	case attribute.EMPTY:
	case attribute.BOOL,
		attribute.INT64,
		attribute.STRING:
		c.Fprint(o, v.String())
	case attribute.FLOAT64:
		v := v.AsFloat64()
		c.Fprintf(o, "%.[1]*g", 3, v)
	case attribute.BYTESLICE:
		bs := v.AsByteSlice()
		n := len(bs)
		if n > maxBytes {
			n = maxBytes
		}
		for i, b := range bs[:n] {
			if i > 0 {
				c.Fprint(o, ".")
			}
			c.Fprintf(o, "%02x", b)
		}
		if len(bs) > maxBytes {
			c.Fprintf(o, "..(+%d)", len(bs)-maxBytes)
		}
	// A slice of one type is a slice all the same, drawn as one of values.
	case attribute.SLICE,
		attribute.BOOLSLICE,
		attribute.INT64SLICE,
		attribute.FLOAT64SLICE,
		attribute.STRINGSLICE:
		vs := elems(v)
		c_bracket.Fprint(o, "[")
		n := len(vs)
		if n > maxSlice {
			n = maxSlice
		}
		for i, sv := range vs[:n] {
			if i > 0 {
				c_faint.Fprint(o, ", ")
			}
			renderAttrValue(o, sv)
		}
		if len(vs) > maxSlice {
			c_faint.Fprintf(o, ", ...(+%d)", len(vs)-maxSlice)
		}
		c_bracket.Fprint(o, "]")
	case attribute.MAP:
		kvs := v.AsMap()
		c_bracket.Fprint(o, "{")
		n := len(kvs)
		if n > maxMap {
			n = maxMap
		}
		for i, kv := range kvs[:n] {
			if i > 0 {
				c_faint.Fprint(o, ", ")
			}
			c_map_key.Fprint(o, kv.Key)
			c_faint.Fprint(o, ":")
			renderAttrValue(o, kv.Value)
		}
		if len(kvs) > maxMap {
			c_faint.Fprintf(o, ", ...(+%d)", len(kvs)-maxMap)
		}
		c_bracket.Fprint(o, "}")
	default:
	}

	return nil
}

// elems is a slice's values, whichever of attribute's slices it is: one of
// values, or one of a single type, which the log API had none of.
func elems(v attribute.Value) []attribute.Value {
	switch v.Type() {
	case attribute.BOOLSLICE:
		return each(v.AsBoolSlice(), attribute.BoolValue)
	case attribute.INT64SLICE:
		return each(v.AsInt64Slice(), attribute.Int64Value)
	case attribute.FLOAT64SLICE:
		return each(v.AsFloat64Slice(), attribute.Float64Value)
	case attribute.STRINGSLICE:
		return each(v.AsStringSlice(), attribute.StringValue)
	default:
		return v.AsSlice()
	}
}

func each[T any](vs []T, f func(T) attribute.Value) []attribute.Value {
	rst := make([]attribute.Value, len(vs))
	for i, v := range vs {
		rst[i] = f(v)
	}

	return rst
}
