package mkot

import (
	"fmt"
	"strconv"
	"strings"
)

// ConsoleFormat turns a line carrying ANSI SGR escapes into the two arguments a
// browser console takes: a format string with `%c` where the styling changes,
// and the CSS for each of those points.
//
//	format, styles := ConsoleFormat("\x1b[31mred\x1b[0m plain")
//	// "%cred%c plain", ["color:#cd3131", ""]
//
// It is here, and exported, because a terminal and a browser console disagree
// about how a coloured line is written and a program that writes one should not
// have to care. Go's stderr in a Wasm module arrives at `console.log` as text
// (`wasm_exec.js` sends it there), so escapes that a terminal would act on are
// printed instead: every line begins `[36m`. A console reads `%c` and a CSS
// string per styled run, which is the same information in the other spelling.
//
// What is understood is what a log line uses: reset, bold, faint, italic,
// underline, strikethrough and their cancels, the sixteen colours, `38;5;n`
// and `38;2;r;g;b` with their `48` counterparts. Any other CSI sequence is
// dropped rather than printed -- a cursor movement means nothing to a console,
// and showing it would be worse than losing it.
//
// The colours are VS Code's terminal palette, which is a choice: ANSI names a
// colour and not a value, and a console has no palette to ask.
func ConsoleFormat(line string) (string, []string) {
	var (
		sb    strings.Builder
		css   []string
		cur   sgr
		shown string // the CSS in effect at the last %c
	)

	// The text since the last escape, written with the style in effect now.
	// Nothing is written for an empty run, so a line of pure escapes is empty
	// rather than a string of styles with nothing between them.
	flush := func(text string) {
		if text == "" {
			return
		}

		if v := cur.css(); v != shown {
			sb.WriteString("%c")
			css = append(css, v)
			shown = v
		}

		// The console reads the format string, so a percent in the log is a
		// directive unless it is doubled.
		sb.WriteString(strings.ReplaceAll(text, "%", "%%"))
	}

	for {
		i := strings.IndexByte(line, 0x1b)
		if i < 0 || i+1 >= len(line) || line[i+1] != '[' {
			flush(line)
			break
		}

		flush(line[:i])

		// The parameters run to the first byte that is not one of them; that
		// byte says which sequence this was.
		rest := line[i+2:]
		j := strings.IndexFunc(rest, func(r rune) bool {
			return !(r >= '0' && r <= '9') && r != ';'
		})
		if j < 0 {
			// Truncated: there is no terminator, so there is no sequence.
			break
		}

		if rest[j] == 'm' {
			cur.apply(rest[:j])
		}

		line = rest[j+1:]
	}

	return sb.String(), css
}

// sgr is what an SGR sequence has said so far.
type sgr struct {
	bold      bool
	faint     bool
	italic    bool
	underline bool
	strike    bool

	fg string
	bg string
}

func (s sgr) css() string {
	var vs []string
	if s.bold {
		vs = append(vs, "font-weight:bold")
	}
	if s.faint {
		// A console has no faint. Opacity is what it means and what reads the
		// same against either background.
		vs = append(vs, "opacity:0.6")
	}
	if s.italic {
		vs = append(vs, "font-style:italic")
	}
	if s.underline && s.strike {
		vs = append(vs, "text-decoration:underline line-through")
	} else if s.underline {
		vs = append(vs, "text-decoration:underline")
	} else if s.strike {
		vs = append(vs, "text-decoration:line-through")
	}
	if s.fg != "" {
		vs = append(vs, "color:"+s.fg)
	}
	if s.bg != "" {
		vs = append(vs, "background-color:"+s.bg)
	}

	return strings.Join(vs, ";")
}

// apply reads one SGR parameter list. An empty list is `ESC[m`, which is reset.
func (s *sgr) apply(params string) {
	if params == "" {
		*s = sgr{}
		return
	}

	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		n, err := strconv.Atoi(ps[i])
		if err != nil {
			continue
		}

		switch {
		case n == 0:
			*s = sgr{}
		case n == 1:
			s.bold = true
		case n == 2:
			s.faint = true
		case n == 3:
			s.italic = true
		case n == 4:
			s.underline = true
		case n == 9:
			s.strike = true
		case n == 22:
			s.bold, s.faint = false, false
		case n == 23:
			s.italic = false
		case n == 24:
			s.underline = false
		case n == 29:
			s.strike = false

		case n == 39:
			s.fg = ""
		case n == 49:
			s.bg = ""

		case n >= 30 && n <= 37:
			s.fg = ansi[n-30]
		case n >= 90 && n <= 97:
			s.fg = ansi[n-90+8]
		case n >= 40 && n <= 47:
			s.bg = ansi[n-40]
		case n >= 100 && n <= 107:
			s.bg = ansi[n-100+8]

		case n == 38 || n == 48:
			v, used := extended(ps[i+1:])
			i += used
			if used == 0 {
				continue
			}
			if n == 38 {
				s.fg = v
			} else {
				s.bg = v
			}
		}
	}
}

// extended reads the tail of a `38`/`48` parameter: `5;n` for the 256-colour
// table or `2;r;g;b` for a literal one. It answers with the colour and how many
// parameters it took, so that the caller can step over them.
func extended(ps []string) (string, int) {
	if len(ps) == 0 {
		return "", 0
	}

	switch ps[0] {
	case "5":
		if len(ps) < 2 {
			return "", 0
		}

		n, err := strconv.Atoi(ps[1])
		if err != nil {
			return "", 0
		}

		return xterm(n), 2

	case "2":
		if len(ps) < 4 {
			return "", 0
		}

		var rgb [3]int
		for i := range rgb {
			v, err := strconv.Atoi(ps[i+1])
			if err != nil {
				return "", 0
			}

			rgb[i] = v
		}

		return fmt.Sprintf("rgb(%d,%d,%d)", rgb[0], rgb[1], rgb[2]), 4
	}

	return "", 0
}

// xterm is the 256-colour table: the sixteen, then a 6×6×6 cube, then a ramp of
// greys. The cube's levels are not evenly spaced -- the first step is the large
// one -- which is why they are written out.
func xterm(n int) string {
	switch {
	case n < 0 || n > 255:
		return ""
	case n < 16:
		return ansi[n]
	case n < 232:
		levels := [6]int{0, 95, 135, 175, 215, 255}
		n -= 16
		return fmt.Sprintf("rgb(%d,%d,%d)", levels[n/36], levels[(n/6)%6], levels[n%6])
	default:
		v := 8 + (n-232)*10
		return fmt.Sprintf("rgb(%d,%d,%d)", v, v, v)
	}
}

// ansi is VS Code's terminal palette, which is a choice and has to be: ANSI
// names a colour rather than giving one, and a browser console has no palette
// of its own to ask.
var ansi = [16]string{
	"#000000", "#cd3131", "#0dbc79", "#e5e510",
	"#2472c8", "#bc3fbc", "#11a8cd", "#e5e5e5",
	"#666666", "#f14c4c", "#23d18b", "#f5f543",
	"#3b8eea", "#d670d6", "#29b8db", "#ffffff",
}
