package mkot_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lesomnus/mkot"
)

func TestConsoleFormat(t *testing.T) {
	tcs := []struct {
		desc   string
		in     string
		format string
		styles []string
	}{
		{
			desc:   "a line with nothing in it stays a line with nothing in it",
			in:     "plain",
			format: "plain",
		},
		{
			desc:   "a colour opens a run and the reset closes it",
			in:     "\x1b[31mred\x1b[0m plain",
			format: "%cred%c plain",
			styles: []string{"color:#cd3131", ""},
		},
		{
			desc:   "the same style twice is one run",
			in:     "\x1b[31ma\x1b[31mb",
			format: "%cab",
			styles: []string{"color:#cd3131"},
		},
		{
			desc:   "attributes accumulate until they are cancelled",
			in:     "\x1b[1;4;33mloud\x1b[24m quiet",
			format: "%cloud%c quiet",
			styles: []string{"font-weight:bold;text-decoration:underline;color:#e5e510", "font-weight:bold;color:#e5e510"},
		},
		{
			desc:   "faint is opacity, because a console has no faint",
			in:     "\x1b[2mfaint",
			format: "%cfaint",
			styles: []string{"opacity:0.6"},
		},
		{
			desc:   "underline and strike are one declaration, not two",
			in:     "\x1b[4;9mboth",
			format: "%cboth",
			styles: []string{"text-decoration:underline line-through"},
		},
		{
			desc:   "a background is its own property",
			in:     "\x1b[41mred behind",
			format: "%cred behind",
			styles: []string{"background-color:#cd3131"},
		},
		{
			desc:   "24-bit colour arrives as it was written",
			in:     "\x1b[38;2;169;255;255mreq",
			format: "%creq",
			styles: []string{"color:rgb(169,255,255)"},
		},
		{
			desc:   "the 256-colour cube is not evenly spaced",
			in:     "\x1b[38;5;196mcube",
			format: "%ccube",
			styles: []string{"color:rgb(255,0,0)"},
		},
		{
			desc:   "the grey ramp is the tail of that table",
			in:     "\x1b[38;5;240mgrey",
			format: "%cgrey",
			styles: []string{"color:rgb(88,88,88)"},
		},
		{
			desc:   "the first sixteen of it are the sixteen",
			in:     "\x1b[38;5;9mbright red",
			format: "%cbright red",
			styles: []string{"color:#f14c4c"},
		},
		{
			desc:   "a colour after an extended one is not eaten by it",
			in:     "\x1b[38;5;9;1mboth",
			format: "%cboth",
			styles: []string{"font-weight:bold;color:#f14c4c"},
		},
		{
			desc:   "a percent is a directive to the console unless it is doubled",
			in:     "100% of \x1b[32m50%",
			format: "100%% of %c50%%",
			styles: []string{"color:#0dbc79"},
		},
		{
			desc: "a sequence that is not SGR is dropped rather than printed",
			// A cursor movement means nothing to a console, and showing the
			// escape would be worse than losing it.
			in:     "a\x1b[2Kb",
			format: "ab",
		},
		{
			desc:   "an empty parameter list is a reset",
			in:     "\x1b[31mred\x1b[mplain",
			format: "%cred%cplain",
			styles: []string{"color:#cd3131", ""},
		},
		{
			desc:   "a line that is only escapes has nothing to show",
			in:     "\x1b[31m\x1b[0m",
			format: "",
		},
		{
			desc:   "a truncated escape takes the rest of the line with it",
			in:     "a\x1b[3",
			format: "a",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			x := require.New(t)

			format, styles := mkot.ConsoleFormat(tc.in)
			x.Equal(tc.format, format)
			x.Equal(tc.styles, styles)

			// Whatever the styling, the console is given one style per `%c` --
			// a mismatch silently shifts every style onto the wrong run.
			n := 0
			for i := 0; i+1 < len(format); i++ {
				if format[i] == '%' && format[i+1] == 'c' {
					n++
					i++
				}
			}
			x.Len(styles, n, "one style per %%c")
		})
	}
}
