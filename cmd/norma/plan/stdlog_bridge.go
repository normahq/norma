package plancmd

import (
	"bytes"
	"io"
	stdlog "log"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func installPlanWebStdLogBridge() func() {
	prevWriter := stdlog.Writer()
	prevFlags := stdlog.Flags()
	prevPrefix := stdlog.Prefix()

	stdlog.SetFlags(0)
	stdlog.SetPrefix("")

	bridgeLogger := log.With().Str("component", "tool.plan_web.launcher").Logger()
	bridgeWriter := newPlanWebStdLogWriter(bridgeLogger)
	stdlog.SetOutput(bridgeWriter)

	return func() {
		_ = bridgeWriter.Close()
		stdlog.SetOutput(prevWriter)
		stdlog.SetFlags(prevFlags)
		stdlog.SetPrefix(prevPrefix)
	}
}

type planWebStdLogWriter struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	logger zerolog.Logger
}

func newPlanWebStdLogWriter(logger zerolog.Logger) *planWebStdLogWriter {
	return &planWebStdLogWriter{logger: logger}
}

func (w *planWebStdLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	total := len(p)
	for len(p) > 0 {
		idx := bytes.IndexByte(p, '\n')
		if idx < 0 {
			_, _ = w.buffer.Write(p)
			break
		}
		_, _ = w.buffer.Write(p[:idx])
		w.flushLocked()
		p = p[idx+1:]
	}
	return total, nil
}

func (w *planWebStdLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushLocked()
	return nil
}

func (w *planWebStdLogWriter) flushLocked() {
	line := strings.TrimSpace(w.buffer.String())
	w.buffer.Reset()
	if line == "" {
		return
	}
	w.logger.Debug().Msg(line)
}

var _ io.WriteCloser = (*planWebStdLogWriter)(nil)
