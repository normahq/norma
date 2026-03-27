package main

import (
	"strings"

	"github.com/normahq/norma/internal/apps/relay"
	"github.com/normahq/norma/internal/logging"
)

type relayLoggingSettings struct {
	level string
	json  bool
}

func resolveRelayLoggingSettings(cfg relay.LoggerConfig, debugFlag, traceFlag bool) relayLoggingSettings {
	level := strings.TrimSpace(cfg.Level)
	if level == "" {
		level = logging.LevelInfo
	}
	if debugFlag {
		level = logging.LevelDebug
	}
	if traceFlag {
		level = logging.LevelTrace
	}

	return relayLoggingSettings{
		level: level,
		json:  !cfg.Pretty,
	}
}

func applyRelayLogging(cfg relay.LoggerConfig) error {
	settings := resolveRelayLoggingSettings(cfg, debug, trace)
	return logging.Init(
		logging.WithLevel(settings.level),
		logging.WithJson(settings.json),
	)
}
