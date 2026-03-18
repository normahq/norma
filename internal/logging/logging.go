// Package logging provides application-wide logging configuration.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	globalOpts Options
)

// Init initializes the global logger for zerolog and configures slog for third-party libraries.
func Init(setters ...OptOptionsSetter) error {
	opts := NewOptions(setters...)
	if err := opts.Validate(); err != nil {
		return fmt.Errorf("validate logging options: %w", err)
	}

	globalOpts = opts

	// 1. Configure zerolog (Primary for the project)
	zlLevel := zerolog.InfoLevel
	switch {
	case opts.trace:
		zlLevel = zerolog.TraceLevel
	case opts.debug:
		zlLevel = zerolog.DebugLevel
	}

	zerolog.SetGlobalLevel(zlLevel)

	var zl zerolog.Logger
	if !opts.json {
		zl = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		})
	} else {
		zl = zerolog.New(os.Stderr)
	}

	zl = zl.With().Timestamp().Logger()
	log.Logger = zl
	zerolog.DefaultContextLogger = &log.Logger

	// 2. Configure slog (Only for third-party libraries)
	var slogLevel slog.Level
	switch {
	case opts.trace:
		slogLevel = slog.LevelDebug - 4 // Custom "Trace" level for slog
	case opts.debug:
		slogLevel = slog.LevelDebug
	default:
		slogLevel = slog.LevelInfo
	}

	var slogHandler slog.Handler
	if !opts.json {
		slogHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slogLevel,
		})
	} else {
		slogHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: slogLevel,
		})
	}

	// Set as default so third-party libs using slog will use this configuration.
	slog.SetDefault(slog.New(slogHandler))

	return nil
}

// DebugEnabled reports whether debug logging is enabled.
func DebugEnabled() bool {
	return globalOpts.debug
}

// TraceEnabled reports whether trace logging is enabled.
func TraceEnabled() bool {
	return globalOpts.trace
}

// Ctx returns the logger associated with the context.
func Ctx(ctx context.Context) *zerolog.Logger {
	return log.Ctx(ctx)
}
