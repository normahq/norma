package logging

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/rs/zerolog"
)

func TestInitDefault(t *testing.T) {
	_ = Init()
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Errorf("expected InfoLevel, got %v", zerolog.GlobalLevel())
	}
	if slog.Default().Handler().Enabled(context.TODO(), slog.LevelDebug) {
		t.Error("expected slog level info, but debug enabled")
	}
	if !slog.Default().Handler().Enabled(context.TODO(), slog.LevelInfo) {
		t.Error("expected slog level info enabled")
	}
	if DebugEnabled() {
		t.Error("expected DebugEnabled() to be false")
	}
	if TraceEnabled() {
		t.Error("expected TraceEnabled() to be false")
	}
}

func TestInitDebug(t *testing.T) {
	_ = Init(WithDebug(true))
	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Errorf("expected DebugLevel, got %v", zerolog.GlobalLevel())
	}
	if !slog.Default().Handler().Enabled(context.TODO(), slog.LevelDebug) {
		t.Error("expected slog level debug enabled")
	}
	if !DebugEnabled() {
		t.Error("expected DebugEnabled() to be true")
	}
	if TraceEnabled() {
		t.Error("expected TraceEnabled() to be false")
	}
}

func TestInitTrace(t *testing.T) {
	_ = Init(WithTrace(true))
	if zerolog.GlobalLevel() != zerolog.TraceLevel {
		t.Errorf("expected TraceLevel, got %v", zerolog.GlobalLevel())
	}
	if !slog.Default().Handler().Enabled(context.TODO(), slog.LevelDebug-4) {
		t.Error("expected slog level trace enabled")
	}
	if TraceEnabled() != true {
		t.Error("expected TraceEnabled() to be true")
	}
}

func TestInitTraceOverridesDebug(t *testing.T) {
	_ = Init(WithDebug(true), WithTrace(true))
	if zerolog.GlobalLevel() != zerolog.TraceLevel {
		t.Errorf("expected TraceLevel, got %v", zerolog.GlobalLevel())
	}
	if !TraceEnabled() {
		t.Error("expected TraceEnabled() to be true")
	}
}

func TestDebugEnabled(t *testing.T) {
	_ = Init(WithDebug(true))
	if !DebugEnabled() {
		t.Error("expected DebugEnabled() to be true when debug=true")
	}

	_ = Init(WithDebug(false))
	if DebugEnabled() {
		t.Error("expected DebugEnabled() to be false when debug=false")
	}
}

func TestTraceEnabled(t *testing.T) {
	_ = Init(WithTrace(true))
	if !TraceEnabled() {
		t.Error("expected TraceEnabled() to be true when trace=true")
	}

	_ = Init(WithTrace(false))
	if TraceEnabled() {
		t.Error("expected TraceEnabled() to be false when trace=false")
	}
}

func TestJSONEnabled(t *testing.T) {
	_ = Init(WithJson(true))
}

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	os.Exit(m.Run())
}
