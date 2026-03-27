package main

import (
	"testing"

	relayapp "github.com/normahq/norma/internal/apps/relay"
	"github.com/normahq/norma/internal/logging"
)

func TestResolveRelayLoggingSettings(t *testing.T) {
	tests := []struct {
		name      string
		cfg       relayapp.LoggerConfig
		debugFlag bool
		traceFlag bool
		wantLevel string
		wantJSON  bool
	}{
		{
			name:      "defaults to info when config level empty",
			cfg:       relayapp.LoggerConfig{Pretty: true},
			wantLevel: logging.LevelInfo,
			wantJSON:  false,
		},
		{
			name:      "uses config level when flags disabled",
			cfg:       relayapp.LoggerConfig{Level: " debug ", Pretty: true},
			wantLevel: logging.LevelDebug,
			wantJSON:  false,
		},
		{
			name:      "debug flag overrides config level",
			cfg:       relayapp.LoggerConfig{Level: logging.LevelWarn, Pretty: false},
			debugFlag: true,
			wantLevel: logging.LevelDebug,
			wantJSON:  true,
		},
		{
			name:      "trace flag overrides debug and config",
			cfg:       relayapp.LoggerConfig{Level: logging.LevelWarn, Pretty: true},
			debugFlag: true,
			traceFlag: true,
			wantLevel: logging.LevelTrace,
			wantJSON:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveRelayLoggingSettings(tc.cfg, tc.debugFlag, tc.traceFlag)
			if got.level != tc.wantLevel {
				t.Fatalf("level = %q, want %q", got.level, tc.wantLevel)
			}
			if got.json != tc.wantJSON {
				t.Fatalf("json = %t, want %t", got.json, tc.wantJSON)
			}
		})
	}
}

func TestApplyRelayLogging_DebugFlagOverridesConfig(t *testing.T) {
	restore := setRelayLogFlagsForTest(t, true, false)
	defer restore()

	if err := applyRelayLogging(relayapp.LoggerConfig{Level: logging.LevelError, Pretty: true}); err != nil {
		t.Fatalf("applyRelayLogging(): %v", err)
	}
	if !logging.DebugEnabled() {
		t.Fatal("DebugEnabled() = false, want true")
	}
	if logging.TraceEnabled() {
		t.Fatal("TraceEnabled() = true, want false")
	}
}

func TestApplyRelayLogging_TraceFlagOverridesDebug(t *testing.T) {
	restore := setRelayLogFlagsForTest(t, true, true)
	defer restore()

	if err := applyRelayLogging(relayapp.LoggerConfig{Level: logging.LevelError, Pretty: true}); err != nil {
		t.Fatalf("applyRelayLogging(): %v", err)
	}
	if !logging.TraceEnabled() {
		t.Fatal("TraceEnabled() = false, want true")
	}
}

func setRelayLogFlagsForTest(t *testing.T, debugFlag, traceFlag bool) func() {
	t.Helper()
	prevDebug, prevTrace := debug, trace
	debug, trace = debugFlag, traceFlag
	return func() {
		debug, trace = prevDebug, prevTrace
	}
}
