package plancmd

import (
	"bytes"
	stdlog "log"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

func TestPlanWebStdLogWriterFlushesByLine(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.DebugLevel)
	w := newPlanWebStdLogWriter(logger)

	if _, err := w.Write([]byte("first line\nsecond line")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "first line") {
		t.Fatalf("buffer = %q, want first line flushed", got)
	}
	if strings.Contains(got, "second line") {
		t.Fatalf("buffer = %q, second line should not flush before close", got)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !strings.Contains(buf.String(), "second line") {
		t.Fatalf("buffer = %q, want second line flushed on close", buf.String())
	}
}

func TestInstallPlanWebStdLogBridgeRoutesStdLogToZeroLog(t *testing.T) {
	var buf bytes.Buffer
	prevLogger := zlog.Logger
	zlog.Logger = zerolog.New(&buf).Level(zerolog.DebugLevel)
	defer func() {
		zlog.Logger = prevLogger
	}()

	restore := installPlanWebStdLogBridge()
	defer restore()

	stdlog.Println("launcher started")

	got := buf.String()
	if !strings.Contains(got, "launcher started") {
		t.Fatalf("buffer = %q, want bridged stdlog line", got)
	}
	if !strings.Contains(got, "tool.plan_web.launcher") {
		t.Fatalf("buffer = %q, want component field", got)
	}
}
