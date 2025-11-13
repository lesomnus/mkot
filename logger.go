package mkot

import (
	"context"

	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/embedded"
)

type multiLoggerProvider struct {
	embedded.LoggerProvider
	providers []log.LoggerProvider
}

func (m multiLoggerProvider) Logger(name string, opts ...log.LoggerOption) log.Logger {
	loggers := make([]log.Logger, len(m.providers))
	for i, p := range m.providers {
		loggers[i] = p.Logger(name, opts...)
	}
	return &multiLogger{loggers: loggers}
}

type multiLogger struct {
	embedded.Logger
	loggers []log.Logger
}

func (l multiLogger) Emit(ctx context.Context, record log.Record) {
	for _, l := range l.loggers {
		l.Emit(ctx, record)
	}
}

func (l multiLogger) Enabled(ctx context.Context, param log.EnabledParameters) bool {
	for _, l := range l.loggers {
		if l.Enabled(ctx, param) {
			return true
		}
	}

	return false
}
