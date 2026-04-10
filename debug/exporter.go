package debug

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/lesomnus/mkot"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

var _ mkot.ExporterConfig = (*Exporter)(nil)

type Exporter struct {
	// OutputPaths is a list of file paths to write logging output to.
	// This option can only be used when use_internal_logger is false.
	// Special strings "stdout" and "stderr" are interpreted as os.Stdout and os.Stderr respectively.
	// All other values are treated as file paths.
	// If not set, defaults to ["stdout"].
	OutputPaths []string `yaml:"output_paths,omitempty"`

	SendingQueue mkot.QueueConfig `yaml:"sending_queue,omitempty"`
}

var writers = map[string]*sharedWriter{
	"stdout": {n: -1, w: os.Stdout},
	"stderr": {n: -1, w: os.Stderr},
}

func (e Exporter) SpanExporter(ctx context.Context) (trace.SpanExporter, []trace.TracerProviderOption, error) {
	w, err := e.open()
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	v, err := stdouttrace.New(stdouttrace.WithWriter(w))
	if err != nil {
		return nil, nil, err
	}

	p := e.SendingQueue.BuildSpanProcessor(v)
	return v, []trace.TracerProviderOption{trace.WithSpanProcessor(p)}, nil
}

func (e Exporter) MetricExporter(ctx context.Context) (metric.Exporter, []metric.Option, error) {
	w, err := e.open()
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	v, err := stdoutmetric.New(stdoutmetric.WithWriter(w))
	if err != nil {
		return nil, nil, err
	}

	return v, []metric.Option{metric.WithReader(metric.NewPeriodicReader(v))}, nil
}

func (e Exporter) LogExporter(ctx context.Context) (log.Exporter, []log.LoggerProviderOption, error) {
	w, err := e.open()
	if err != nil {
		return nil, nil, fmt.Errorf("open: %w", err)
	}

	v, err := stdoutlog.New(stdoutlog.WithWriter(w))
	if err != nil {
		return nil, nil, err
	}

	p := e.SendingQueue.BuildLogProcessor(v)
	return v, []log.LoggerProviderOption{log.WithProcessor(p)}, nil
}

// Close can be concurrently called but open is not concurrent safe.
func (e Exporter) open() (io.WriteCloser, error) {
	var ws []io.WriteCloser
	for _, p := range e.OutputPaths {
		w, ok := writers[p]
		if !ok || w.n == 0 {
			f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
			if err != nil {
				// Clenan up already opened writers before returning error.
				for _, w := range ws {
					w.Close()
				}
				return nil, fmt.Errorf("open file %s: %w", p, err)
			}
			w = &sharedWriter{n: 0, w: f}
		}
		if w.n > -1 {
			w.n++
		}
		ws = append(ws, w)
	}
	return newMultiWriter(ws), nil
}

type sharedWriter struct {
	mu sync.Mutex
	n  int // num of writers
	w  io.WriteCloser
}

func (s *sharedWriter) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.w.Write(p)
}

func (s *sharedWriter) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.n != 0 {
		s.n--
		return nil
	}
	return s.w.Close()
}

type multiWriter struct {
	writers []io.WriteCloser
}

func (m *multiWriter) Write(p []byte) (n int, err error) {
	for _, w := range m.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
		if n != len(p) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(p), nil
}

func (m *multiWriter) Close() error {
	var err error
	for _, w := range m.writers {
		if e := w.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

func newMultiWriter(writers []io.WriteCloser) io.WriteCloser {
	return &multiWriter{writers: writers}
}
