package pretty

import (
	"io"

	"go.opentelemetry.io/otel/log"
)

func renderAttrValue(o io.Writer, v log.Value) error {
	const (
		maxBytes = 8
		maxSlice = 5
		maxMap   = 5
	)

	c := attr_colors[v.Kind()]
	switch v.Kind() {
	case log.KindEmpty:
	case log.KindBool,
		log.KindInt64,
		log.KindString:
		c.Fprint(o, v.String())
	case log.KindFloat64:
		v := v.AsFloat64()
		c.Fprintf(o, "%.[1]*g", 3, v)
	case log.KindBytes:
		bs := v.AsBytes()
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
	case log.KindSlice:
		vs := v.AsSlice()
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
	case log.KindMap:
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
