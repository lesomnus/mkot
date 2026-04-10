package mkot

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/lesomnus/mkot/internal/x"
	"go.opentelemetry.io/otel/attribute"
)

func TestAttrMarshal(t *testing.T) {
	_, x := x.New(t)

	a := Attr{
		Key:   "foo",
		Value: attribute.StringValue("bar"),
	}

	v, err := yaml.Marshal(a)
	x.NoError(err)
	x.Eq("key: foo\nvalue: bar\n", string(v))
}

func TestAttrUnmarshal(t *testing.T) {
	_, x := x.New(t)

	for _, tc := range []struct {
		given    string
		expected attribute.Value
	}{
		{"bar", attribute.StringValue("bar")},
		{"42", attribute.IntValue(42)},
		{"3.14", attribute.Float64Value(3.14)},
		{"true", attribute.BoolValue(true)},

		{"[\"a\", \"b\", \"c\"]", attribute.StringSliceValue([]string{"a", "b", "c"})},
		{"[1, 2, 3]", attribute.IntSliceValue([]int{1, 2, 3})},
		{"[1.1, 2.2, 3.3]", attribute.Float64SliceValue([]float64{1.1, 2.2, 3.3})},
		{"[true, false, true]", attribute.BoolSliceValue([]bool{true, false, true})},
	} {
		var a Attr
		err := yaml.Unmarshal([]byte("key: foo\nvalue: "+tc.given), &a)
		x.NoError(err)
		x.Eq(tc.expected, a.Value)
	}
}
