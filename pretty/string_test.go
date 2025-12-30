package pretty

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/stretchr/testify/require"
)

func TestRenderDuration(t *testing.T) {
	origin := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = origin }()

	cs := []struct {
		s string
		d time.Duration
	}{
		{s: "000.00µs", d: time.Nanosecond * 1},
		{s: "000.02µs", d: time.Nanosecond * 20},
		{s: "000.02µs", d: time.Nanosecond * 21},
		{s: "000.32µs", d: time.Nanosecond * 320},
		{s: "000.32µs", d: time.Nanosecond * 321},
		{s: "004.00µs", d: time.Nanosecond * 4_000},
		{s: "004.30µs", d: time.Nanosecond * 4_300},
		{s: "004.32µs", d: time.Nanosecond * 4_320},
		{s: "004.32µs", d: time.Nanosecond * 4_321},
		{s: "054.00µs", d: time.Nanosecond * 54_000},
		{s: "054.32µs", d: time.Nanosecond * 54_321},
		{s: "654.32µs", d: time.Nanosecond * 654_321},
		{s: "007.65ms", d: time.Nanosecond * 7_654_321},
		{s: "087.65ms", d: time.Nanosecond * 87_654_321},
		{s: "987.65ms", d: time.Nanosecond * 987_654_321},
		{s: "999.99ms", d: time.Nanosecond * 999_999_999},

		{s: "00m04.0s", d: time.Millisecond * 4_000},
		{s: "00m04.3s", d: time.Millisecond * 4_300},
		{s: "00m04.3s", d: time.Millisecond * 4_320},
		{s: "00m04.3s", d: time.Millisecond * 4_321},
		{s: "00m54.0s", d: time.Millisecond * 54_000},
		{s: "00m54.3s", d: time.Millisecond * 54_321},
		{s: "00m59.9s", d: time.Millisecond * 59_999},
		{s: "01m00.0s", d: time.Millisecond * 60_000},
		{s: "06m00.0s", d: time.Minute * 6},
		{s: "06m50.0s", d: time.Minute*6 + time.Millisecond*50_000},
		{s: "06m54.0s", d: time.Minute*6 + time.Millisecond*54_000},
		{s: "06m54.3s", d: time.Minute*6 + time.Millisecond*54_321},
		{s: "16m54.3s", d: time.Minute*16 + time.Millisecond*54_321},
		{s: "59m59.9s", d: time.Minute*59 + time.Millisecond*59_999},

		{s: "00d01:00", d: time.Minute * 60},
		{s: "00d03:21", d: time.Hour*3 + time.Minute*21},
		{s: "00d23:21", d: time.Hour*23 + time.Minute*21},
		{s: "00d23:59", d: time.Hour*23 + time.Minute*59 + time.Second*59},
		{s: "01d00:00", d: time.Hour * 24},
		{s: "01d03:21", d: time.Hour*24 + time.Hour*3 + time.Minute*21},
		{s: "01d23:21", d: time.Hour*24 + time.Hour*23 + time.Minute*21},
		{s: "01d23:59", d: time.Hour*24 + time.Hour*23 + time.Minute*59 + time.Second*59},
		{s: "02d00:00", d: time.Hour * 24 * 2},
		{s: "99d00:00", d: time.Hour * 24 * 99},
		{s: "99d23:59", d: time.Hour*24*99 + time.Hour*23 + time.Minute*59 + time.Second*59},

		{s: "0100d00h", d: time.Hour * 24 * 100},
		{s: "0100d23h", d: time.Hour*24*100 + time.Hour*23 + time.Minute*59 + time.Second*59},
		{s: "0101d00h", d: time.Hour*24*100 + time.Hour*24},
		{s: "6543d21h", d: time.Hour*24*6543 + time.Hour*21},
		{s: "9999d23h", d: time.Hour*24*9999 + time.Hour*23 + time.Minute*59 + time.Second*59},

		{s: "TOO_LONG", d: time.Hour*24*9999 + time.Hour*24},
	}
	for _, c := range cs {
		b := &bytes.Buffer{}
		renderDuration(b, c.d)
		require.Equal(t, c.s, b.String(), c.s)
	}
}

func TestHumanizeBytes(t *testing.T) {
	origin := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = origin }()

	const K = 1024
	cs := []struct {
		s string
		n int64
	}{
		{s: "0000B", n: 0},
		{s: "0001B", n: 1},
		{s: "0021B", n: 21},
		{s: "0321B", n: 321},
		{s: "1023B", n: 1023},
		{s: "1024B", n: 1024},
		{s: "9999B", n: 9999},
		{s: "0009K", n: 10000},
		{s: "0009K", n: 10001},
		{s: "0009K", n: 9*K + (K - 1)},
		{s: "0010K", n: 10 * K},
		{s: "9999K", n: 9999 * K},
		{s: "0009M", n: 10000 * K},
		{s: "0009M", n: 10001 * K},
	}
	for _, c := range cs {
		b := &bytes.Buffer{}
		humanizeBytes(b, c.n)
		require.Equal(t, c.s, b.String(), c.s)
	}
}
