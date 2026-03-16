// Package logging provides application-wide logging configuration.
package logging

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var debugEnabled bool
var traceEnabled bool

// Init initializes the global logger.
func Init(debug, trace bool) {
	debugEnabled = debug
	traceEnabled = trace
	level := zerolog.InfoLevel
	if trace {
		level = zerolog.TraceLevel
	} else if debug {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)
	log.Logger = zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	}).With().Timestamp().Logger()
}

// DebugEnabled reports whether debug logging is enabled.
func DebugEnabled() bool {
	return debugEnabled
}

// TraceEnabled reports whether trace logging is enabled.
func TraceEnabled() bool {
	return traceEnabled
}
