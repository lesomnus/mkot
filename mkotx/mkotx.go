// Package mkotx builds a [github.com/lesomnus/otx.Otx] from a [mkot.Resolver],
// so that a configuration file is all it takes to get telemetry into a
// context.
//
//	x, err := mkotx.FromConfig(ctx, c, "")
//	if err != nil {
//		return err
//	}
//	if err := x.Start(ctx); err != nil {
//		return err
//	}
//	defer x.Shutdown(ctx)
//
//	ctx = otx.Into(ctx, x)
//
// The Resolver becomes the [github.com/lesomnus/otx.Controller] of the Otx, so
// x.Start starts the components it built and x.Shutdown stops them - after the
// providers have flushed into them, which is the order otx shuts down in.
//
// It is a module of its own because otx is of no use to a program that only
// reads a configuration, and mkot is of no use to one that is handed its
// providers by something else.
package mkotx

import (
	"context"
	"errors"
	"fmt"

	"github.com/lesomnus/mkot"
	"github.com/lesomnus/otx"
)

// A Resolver is a Controller: this is what lets the Otx drive the lifecycle of
// everything the configuration described.
var _ otx.Controller = (mkot.Resolver)(nil)

// New builds an Otx from the providers r resolves under the given name, which
// is empty for the ones a configuration declares without one.
//
// A signal the configuration does not mention resolves to [mkot.ErrNotExist],
// which is not an error here: that signal is simply off, and the no-op provider
// r hands back alongside it is used. Any other failure is returned, and no Otx
// with it.
//
// The providers are not started. Give the Otx to the program and call
// [github.com/lesomnus/otx.Otx.Start], or hold r and call r.Start; they are the
// same call.
func New(ctx context.Context, r mkot.Resolver, name string, opts ...otx.Option) (*otx.Otx, error) {
	if r == nil {
		return nil, errors.New("mkotx: New called with a nil Resolver")
	}

	tracer_provider, err := r.Tracer(ctx, name)
	if err != nil && !errors.Is(err, mkot.ErrNotExist) {
		return nil, fmt.Errorf("mkotx: resolve tracer provider: %w", err)
	}

	meter_provider, err := r.Meter(ctx, name)
	if err != nil && !errors.Is(err, mkot.ErrNotExist) {
		return nil, fmt.Errorf("mkotx: resolve meter provider: %w", err)
	}

	logger_provider, err := r.Logger(ctx, name)
	if err != nil && !errors.Is(err, mkot.ErrNotExist) {
		return nil, fmt.Errorf("mkotx: resolve logger provider: %w", err)
	}

	// The options given by the caller come last so that they win, which is how
	// otx.New reads them.
	return otx.New(append([]otx.Option{
		otx.WithController(r),
		otx.WithTracerProvider(tracer_provider),
		otx.WithMeterProvider(meter_provider),
		otx.WithLoggerProvider(logger_provider),
	}, opts...)...), nil
}

// FromConfig makes a Resolver from c and builds an Otx from it, which is what
// a program that has just read its configuration wants.
//
// Use [mkot.Make] and [New] instead when the Resolver is needed afterwards, to
// resolve a second provider under another name.
func FromConfig(ctx context.Context, c *mkot.Config, name string, opts ...otx.Option) (*otx.Otx, error) {
	return New(ctx, mkot.Make(ctx, c), name, opts...)
}
