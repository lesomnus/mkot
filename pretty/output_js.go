//go:build js

package pretty

import "github.com/fatih/color"

// defaultOutputPaths is where a line goes when nothing said.
//
// Not `stderr`, which is a real place in a Wasm module and the wrong one: it
// reaches the browser through `wasm_exec.js`, as text, so a coloured line
// arrives with `[36m` printed in the middle of it. `console` is the same line
// spelled the way a console reads it; see `mkot`'s console writer.
//
// Anybody who names an output still gets what they named.
func defaultOutputPaths() []string { return []string{"console"} }

// A terminal is what `fatih/color` looks for at init, and there is none here,
// so it decides once and for all that nothing is coloured.
//
// That is the right answer for a pipe and the wrong one here: the console this
// writes to does paint, it just wants CSS rather than escapes, and the writer
// on the other end reads the escapes to produce it. Turning colour off would
// leave that writer with nothing to translate.
//
// It is set here rather than asked of every program, because a program that
// imports this package has said what it wants its logs to look like.
func init() { color.NoColor = false }
