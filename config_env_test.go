package mkot_test

import (
	"testing"

	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/debug"
	"github.com/lesomnus/mkot/internal/x"
)

func TestExpandEnv(t *testing.T) {
	_, x := x.New(t)
	t.Setenv("MKOT_ENDPOINT", "collector:4317")
	t.Setenv("MKOT_EMPTY", "")

	out, err := mkot.ExpandEnv([]byte(`
a: ${env:MKOT_ENDPOINT}
b: ${env:MKOT_MISSING:-fallback}
c: ${env:MKOT_EMPTY:-fallback}
d: $${env:MKOT_ENDPOINT}
`))
	x.NoError(err)
	got := string(out)
	x.Eq(true, contains(got, "a: collector:4317"))
	x.Eq(true, contains(got, "b: fallback")) // unset → default
	x.Eq(true, contains(got, "c: "))         // set-but-empty → the empty value, not the default
	x.Eq(false, contains(got, "c: fallback"))
	x.Eq(true, contains(got, "d: ${env:MKOT_ENDPOINT}")) // $$ escapes expansion
}

func TestExpandEnvMissingIsError(t *testing.T) {
	if _, err := mkot.ExpandEnv([]byte(`a: ${env:MKOT_DEFINITELY_UNSET}`)); err == nil {
		t.Fatal("an unset variable with no default must error, not resolve to empty")
	}
}

func TestLoadExpandsThenUnmarshals(t *testing.T) {
	_, x := x.New(t)
	t.Setenv("MKOT_OUT", "/var/log/otel.json")

	// The debug exporter (same module) is registered via its import; use it so
	// the expanded value lands in a real exporter config.
	c, err := mkot.Load([]byte(`
exporters:
  debug:
    output_paths: ["${env:MKOT_OUT}"]
providers:
  tracer:
    exporters: [debug]
`))
	x.NoError(err)
	e, ok := c.Exporters[mkot.Id("debug")].(*debug.ExporterConfig)
	x.Eq(true, ok)
	x.Eq(1, len(e.OutputPaths))
	x.Eq("/var/log/otel.json", e.OutputPaths[0])
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
