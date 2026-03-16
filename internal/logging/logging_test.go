package logging

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
)

func TestInitDefault(t *testing.T) {
	Init(false, false)
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Errorf("expected InfoLevel, got %v", zerolog.GlobalLevel())
	}
	if DebugEnabled() {
		t.Error("expected DebugEnabled() to be false")
	}
	if TraceEnabled() {
		t.Error("expected TraceEnabled() to be false")
	}
}

func TestInitDebug(t *testing.T) {
	Init(true, false)
	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Errorf("expected DebugLevel, got %v", zerolog.GlobalLevel())
	}
	if !DebugEnabled() {
		t.Error("expected DebugEnabled() to be true")
	}
	if TraceEnabled() {
		t.Error("expected TraceEnabled() to be false")
	}
}

func TestInitTrace(t *testing.T) {
	Init(false, true)
	if zerolog.GlobalLevel() != zerolog.TraceLevel {
		t.Errorf("expected TraceLevel, got %v", zerolog.GlobalLevel())
	}
	if TraceEnabled() != true {
		t.Error("expected TraceEnabled() to be true")
	}
}

func TestInitTraceOverridesDebug(t *testing.T) {
	Init(true, true)
	if zerolog.GlobalLevel() != zerolog.TraceLevel {
		t.Errorf("expected TraceLevel, got %v", zerolog.GlobalLevel())
	}
	if !TraceEnabled() {
		t.Error("expected TraceEnabled() to be true")
	}
}

func TestDebugEnabled(t *testing.T) {
	Init(true, false)
	if !DebugEnabled() {
		t.Error("expected DebugEnabled() to be true when debug=true")
	}

	Init(false, false)
	if DebugEnabled() {
		t.Error("expected DebugEnabled() to be false when debug=false")
	}
}

func TestTraceEnabled(t *testing.T) {
	Init(false, true)
	if !TraceEnabled() {
		t.Error("expected TraceEnabled() to be true when trace=true")
	}

	Init(false, false)
	if TraceEnabled() {
		t.Error("expected TraceEnabled() to be false when trace=false")
	}
}

func TestConsoleWriter(t *testing.T) {
	Init(false, false)

	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Error("expected logger to be initialized with InfoLevel")
	}
}

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	os.Exit(m.Run())
}
