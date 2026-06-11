package pretty

import (
	"bytes"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log"
)

func TestRenderAttrValue(t *testing.T) {
	origin := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = origin }()

	render := func(v log.Value) string {
		b := &bytes.Buffer{}
		renderAttrValue(b, v)
		return b.String()
	}

	t.Run("Empty", func(t *testing.T) {
		require.Equal(t, "", render(log.Value{}))
	})
	t.Run("Bool", func(t *testing.T) {
		require.Equal(t, "true", render(log.BoolValue(true)))
		require.Equal(t, "false", render(log.BoolValue(false)))
	})
	t.Run("Int64", func(t *testing.T) {
		require.Equal(t, "42", render(log.Int64Value(42)))
		require.Equal(t, "-7", render(log.Int64Value(-7)))
	})
	t.Run("String", func(t *testing.T) {
		require.Equal(t, "hello", render(log.StringValue("hello")))
	})
	t.Run("Float64", func(t *testing.T) {
		require.Equal(t, "3.14", render(log.Float64Value(3.14)))
		require.Equal(t, "1e+10", render(log.Float64Value(1e10)))
	})
	t.Run("Bytes", func(t *testing.T) {
		require.Equal(t, "", render(log.BytesValue(nil)))
		require.Equal(t, "de", render(log.BytesValue([]byte{0xde})))
		require.Equal(t, "de.ad.be.ef", render(log.BytesValue([]byte{0xde, 0xad, 0xbe, 0xef})))

		exactly8 := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
		require.Equal(t, "00.01.02.03.04.05.06.07", render(log.BytesValue(exactly8)))

		over8 := append(exactly8, 0x08)
		require.Equal(t, "00.01.02.03.04.05.06.07..(+1)", render(log.BytesValue(over8)))
	})
	t.Run("Slice", func(t *testing.T) {
		require.Equal(t, "[]", render(log.SliceValue()))
		require.Equal(t, "[true]", render(log.SliceValue(log.BoolValue(true))))
		require.Equal(t, "[1, 2, 3]", render(log.SliceValue(
			log.Int64Value(1), log.Int64Value(2), log.Int64Value(3),
		)))

		exactly5 := log.SliceValue(
			log.Int64Value(1), log.Int64Value(2), log.Int64Value(3),
			log.Int64Value(4), log.Int64Value(5),
		)
		require.Equal(t, "[1, 2, 3, 4, 5]", render(exactly5))

		over5 := log.SliceValue(
			log.Int64Value(1), log.Int64Value(2), log.Int64Value(3),
			log.Int64Value(4), log.Int64Value(5), log.Int64Value(6),
		)
		require.Equal(t, "[1, 2, 3, 4, 5, ...(+1)]", render(over5))
	})
	t.Run("Map", func(t *testing.T) {
		require.Equal(t, "{}", render(log.MapValue()))
		require.Equal(t, "{k:v}", render(log.MapValue(log.String("k", "v"))))
		require.Equal(t, "{a:1, b:2}", render(log.MapValue(
			log.Int("a", 1), log.Int("b", 2),
		)))

		exactly5 := log.MapValue(
			log.Int("a", 1), log.Int("b", 2), log.Int("c", 3),
			log.Int("d", 4), log.Int("e", 5),
		)
		require.Equal(t, "{a:1, b:2, c:3, d:4, e:5}", render(exactly5))

		over5 := log.MapValue(
			log.Int("a", 1), log.Int("b", 2), log.Int("c", 3),
			log.Int("d", 4), log.Int("e", 5), log.Int("f", 6),
		)
		require.Equal(t, "{a:1, b:2, c:3, d:4, e:5, ...(+1)}", render(over5))
	})
	t.Run("Slice/nested", func(t *testing.T) {
		v := log.SliceValue(log.MapValue(log.String("x", "y")))
		require.Equal(t, "[{x:y}]", render(v))
	})
}
