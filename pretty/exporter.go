package pretty

import (
	"context"
	"io"

	"github.com/lesomnus/mkot"
	"go.opentelemetry.io/otel/sdk/log"
)

var _ mkot.ExporterConfig = (*ExporterConfig)(nil)

type ExporterConfig struct {
	mkot.UnimplementedExporterConfig

	// OutputPaths is a list of file paths to write logging output to.
	// This option can only be used when use_internal_logger is false.
	// Special strings "stdout" and "stderr" are interpreted as os.Stdout and os.Stderr respectively.
	// All other values are treated as file paths.
	// If not set, defaults to ["stderr"].
	OutputPaths []string `yaml:"output_paths,omitempty"`

	// Outputs is a list of functions that opens [io.WriteCloser]s to write logging output to.
	Outputs []mkot.WriterOpenFunc `yaml:"-"`
}

func (e ExporterConfig) LogExporter(ctx context.Context) (log.Exporter, []log.LoggerProviderOption, error) {
	w, err := e.open()
	if err != nil {
		return nil, nil, err
	}

	v := &LogExporter{w}
	p := log.NewSimpleProcessor(v)
	return v, []log.LoggerProviderOption{log.WithProcessor(p)}, nil
}

func (e ExporterConfig) open() (io.WriteCloser, error) {
	ps := e.OutputPaths
	if len(ps) == 0 {
		ps = []string{"stderr"}
	}

	ws, err := mkot.Outputs.OpenAll(ps)
	if err != nil {
		return nil, err
	}

	for _, open := range e.Outputs {
		w, err := open()
		if err != nil {
			ws.Close()
			return nil, err
		}
		ws = append(ws, w)
	}

	return ws, nil
}

func init() {
	mkot.DefaultExporterRegistry.Set("pretty", func() mkot.ExporterConfig {
		return &ExporterConfig{}
	})
}
