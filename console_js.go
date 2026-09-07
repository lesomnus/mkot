//go:build js && wasm

package mkot

import (
	"bytes"
	"io"
	"sync"
	"syscall/js"
)

// "console" is an output on Wasm, and it is what a program in a page wants.
//
// Go's stderr does reach the browser: `wasm_exec.js` collects writes to fd 2
// and calls `console.log` with the text. What it does not do is act on what is
// in the text, so a line a terminal would paint arrives with `[36m` in the
// middle of it -- legible, in the way a stack trace is legible.
//
// A console styles a line the other way round: `console.log` takes a format
// string with `%c` in it and one CSS string per `%c`. [ConsoleFormat] is the
// translation. This writer holds a line until it is complete, because a run of
// styling can be split across two `Write` calls and half an escape sequence is
// not a colour.
//
// Registered here rather than left to each program because there is one right
// answer and it takes 150 lines to write again:
//
//	pretty.ExporterConfig{OutputPaths: []string{"console"}}
//
// and on Wasm that is already the default; see the `pretty` package.
func init() {
	Outputs["console"] = NewSharedWriter(func() (io.WriteCloser, error) {
		return &consoleWriter{}, nil
	})
}

type consoleWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write keeps whatever is not yet a line, and logs each one that is.
//
// A partial line is held rather than logged, since `console.log` puts every
// call on its own row: flushing at the boundary a caller happened to write at
// would break one log record across several, each with the styling of the run
// it started in.
func (c *consoleWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.buf.Write(p)
	for {
		line, err := c.buf.ReadString('\n')
		if err != nil {
			// Not a line yet. What was read has to go back, since ReadString
			// consumed it.
			c.buf.Reset()
			c.buf.WriteString(line)

			break
		}

		emit(line[:len(line)-1])
	}

	return len(p), nil
}

// Close writes the last line, which will not have ended in a newline if the
// program is going down mid-record.
func (c *consoleWriter) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.buf.Len() > 0 {
		emit(c.buf.String())
		c.buf.Reset()
	}

	return nil
}

func emit(line string) {
	format, styles := ConsoleFormat(line)

	args := make([]any, 0, len(styles)+1)
	args = append(args, format)
	for _, v := range styles {
		args = append(args, v)
	}

	js.Global().Get("console").Call("log", args...)
}
