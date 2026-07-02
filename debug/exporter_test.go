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
	c.Exporters["debug"] = ExporterConfig{
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

// With an explicit sending_queue opt-in the pipeline batches; the record must
// then be flushed by the component's Shutdown, not silently dropped.
func TestExporterQueueFlushOnShutdown(t *testing.T) {
	ctx, x := x.New(t)

	temp := t.TempDir()
	f, err := os.CreateTemp(temp, "")
	x.NoError(err)

	enabled := true
	c := mkot.NewConfig()
	c.Exporters["debug"] = ExporterConfig{
		OutputPaths: []string{f.Name()},
		Queue:       mkot.QueueConfig{Enabled: &enabled},
	}
	c.Providers["logger/debug"] = &mkot.ProviderConfig{
		Exporters: []mkot.Id{"debug"},
	}
	resolver := mkot.Make(ctx, c)
	err = resolver.Start(ctx)
	x.NoError(err)

	lp, err := resolver.Logger(ctx, "debug")
	x.NoError(err)

	record := log.Record{}
	record.SetBody(log.StringValue("batched"))
	lp.Logger("").Emit(ctx, record)

	err = resolver.Shutdown(ctx)
	x.NoError(err)

	data, err := os.ReadFile(f.Name())
	x.NoError(err)
	x.Contains(string(data), `"Value":"batched"`)
}

// Explicit queue settings imply the batching opt-in even without enabled: true;
// they must not be silently discarded.
func TestExporterQueueSettingsImplyOptIn(t *testing.T) {
	ctx, x := x.New(t)

	temp := t.TempDir()
	f, err := os.CreateTemp(temp, "")
	x.NoError(err)

	c := mkot.NewConfig()
	c.Exporters["debug"] = ExporterConfig{
		OutputPaths: []string{f.Name()},
		Queue:       mkot.QueueConfig{QueueSize: 16},
	}
	c.Providers["logger/debug"] = &mkot.ProviderConfig{
		Exporters: []mkot.Id{"debug"},
	}
	resolver := mkot.Make(ctx, c)
	err = resolver.Start(ctx)
	x.NoError(err)

	lp, err := resolver.Logger(ctx, "debug")
	x.NoError(err)

	record := log.Record{}
	record.SetBody(log.StringValue("queued"))
	lp.Logger("").Emit(ctx, record)

	// Batched: not on disk yet; the shutdown flush delivers it.
	err = resolver.Shutdown(ctx)
	x.NoError(err)

	data, err := os.ReadFile(f.Name())
	x.NoError(err)
	x.Contains(string(data), `"Value":"queued"`)
}
