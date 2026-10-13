package pretty

import (
	"bytes"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func TestRenderAttrValue(t *testing.T) {
	origin := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = origin }()

	render := func(v attribute.Value) string {
		b := &bytes.Buffer{}
		renderAttrValue(b, v)
		return b.String()
	}

	t.Run("Empty", func(t *testing.T) {
		require.Equal(t, "", render(attribute.Value{}))
	})
	t.Run("Bool", func(t *testing.T) {
		require.Equal(t, "true", render(attribute.BoolValue(true)))
		require.Equal(t, "false", render(attribute.BoolValue(false)))
	})
	t.Run("Int64", func(t *testing.T) {
		require.Equal(t, "42", render(attribute.Int64Value(42)))
		require.Equal(t, "-7", render(attribute.Int64Value(-7)))
	})
	t.Run("String", func(t *testing.T) {
		require.Equal(t, "hello", render(attribute.StringValue("hello")))
	})
	t.Run("Float64", func(t *testing.T) {
		require.Equal(t, "3.14", render(attribute.Float64Value(3.14)))
		require.Equal(t, "1e+10", render(attribute.Float64Value(1e10)))
	})
	t.Run("Bytes", func(t *testing.T) {
		require.Equal(t, "", render(attribute.ByteSliceValue(nil)))
		require.Equal(t, "de", render(attribute.ByteSliceValue([]byte{0xde})))
		require.Equal(t, "de.ad.be.ef", render(attribute.ByteSliceValue([]byte{0xde, 0xad, 0xbe, 0xef})))

		exactly8 := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
		require.Equal(t, "00.01.02.03.04.05.06.07", render(attribute.ByteSliceValue(exactly8)))

		over8 := append(exactly8, 0x08)
		require.Equal(t, "00.01.02.03.04.05.06.07..(+1)", render(attribute.ByteSliceValue(over8)))
	})
	// Slices of one type are attribute's own, and the log API had none: each
	// is drawn as the slice of values it holds, cut at the same length.
	t.Run("TypedSlice", func(t *testing.T) {
		require.Equal(t, "[true, false]", render(attribute.BoolSliceValue([]bool{true, false})))
		require.Equal(t, "[1, 2, 3]", render(attribute.Int64SliceValue([]int64{1, 2, 3})))
		require.Equal(t, "[1.5, 2.25]", render(attribute.Float64SliceValue([]float64{1.5, 2.25})))
		require.Equal(t, "[a, b]", render(attribute.StringSliceValue([]string{"a", "b"})))
		require.Equal(t, "[1, 2, 3, 4, 5, ...(+1)]", render(attribute.Int64SliceValue([]int64{1, 2, 3, 4, 5, 6})))
	})
	t.Run("Slice", func(t *testing.T) {
		require.Equal(t, "[]", render(attribute.SliceValue()))
		require.Equal(t, "[true]", render(attribute.SliceValue(attribute.BoolValue(true))))
		require.Equal(t, "[1, 2, 3]", render(attribute.SliceValue(
			attribute.Int64Value(1), attribute.Int64Value(2), attribute.Int64Value(3),
		)))

		exactly5 := attribute.SliceValue(
			attribute.Int64Value(1), attribute.Int64Value(2), attribute.Int64Value(3),
			attribute.Int64Value(4), attribute.Int64Value(5),
		)
		require.Equal(t, "[1, 2, 3, 4, 5]", render(exactly5))

		over5 := attribute.SliceValue(
			attribute.Int64Value(1), attribute.Int64Value(2), attribute.Int64Value(3),
			attribute.Int64Value(4), attribute.Int64Value(5), attribute.Int64Value(6),
		)
		require.Equal(t, "[1, 2, 3, 4, 5, ...(+1)]", render(over5))
	})
	t.Run("Map", func(t *testing.T) {
		require.Equal(t, "{}", render(attribute.MapValue()))
		require.Equal(t, "{k:v}", render(attribute.MapValue(attribute.String("k", "v"))))
		require.Equal(t, "{a:1, b:2}", render(attribute.MapValue(
			attribute.Int("a", 1), attribute.Int("b", 2),
		)))

		exactly5 := attribute.MapValue(
			attribute.Int("a", 1), attribute.Int("b", 2), attribute.Int("c", 3),
			attribute.Int("d", 4), attribute.Int("e", 5),
		)
		require.Equal(t, "{a:1, b:2, c:3, d:4, e:5}", render(exactly5))

		over5 := attribute.MapValue(
			attribute.Int("a", 1), attribute.Int("b", 2), attribute.Int("c", 3),
			attribute.Int("d", 4), attribute.Int("e", 5), attribute.Int("f", 6),
		)
		require.Equal(t, "{a:1, b:2, c:3, d:4, e:5, ...(+1)}", render(over5))
	})
	t.Run("Slice/nested", func(t *testing.T) {
		v := attribute.SliceValue(attribute.MapValue(attribute.String("x", "y")))
		require.Equal(t, "[{x:y}]", render(v))
	})
}
