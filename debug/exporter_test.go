package debug

import (
	"os"
	"testing"

	"github.com/lesomnus/mkot"
	"github.com/lesomnus/mkot/internal/x"
	"go.opentelemetry.io/otel/log"
)

func TestExporter(t *testing.T) {
	ctx, x := x.New(t)

	temp := t.TempDir()
	f, err := os.CreateTemp(temp, "")
	x.NoError(err)

	c := mkot.NewConfig()
	c.Exporters["debug"] = Exporter{
		OutputPaths: []string{f.Name()},
	}
	c.Providers["logger/debug"] = &mkot.ProviderConfig{
		Exporters: []mkot.Id{"debug"},
	}
	resolver := mkot.Make(ctx, c)
	err = resolver.Start(ctx)
	x.NoError(err)
	defer resolver.Shutdown(ctx)

	lp, err := resolver.Logger(ctx, "debug")
	x.NoError(err)

	record := log.Record{}
	record.SetSeverity(log.SeverityWarn)
	record.SetBody(log.StringValue("foo"))

	l := lp.Logger("")
	l.Emit(ctx, record)

	data, err := os.ReadFile(f.Name())
	x.NoError(err)
	x.Contains(string(data), `"Value":"foo"`)
}
