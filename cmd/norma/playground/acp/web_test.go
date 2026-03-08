package acpcmd

import (
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestBuildWebLauncherArgsDefaults(t *testing.T) {
	got := buildWebLauncherArgs(nil)
	want := []string{"web", "api", "webui"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("buildWebLauncherArgs(nil) = %v, want %v", got, want)
	}
}

func TestBuildWebLauncherArgsCustom(t *testing.T) {
	got := buildWebLauncherArgs([]string{"api", "--port", "9999"})
	want := []string{"web", "api", "--port", "9999"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("buildWebLauncherArgs(custom) = %v, want %v", got, want)
	}
}

func TestForceGlobalDebugLogging(t *testing.T) {
	original := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	defer zerolog.SetGlobalLevel(original)

	restore := forceGlobalDebugLogging()
	if got := zerolog.GlobalLevel(); got != zerolog.DebugLevel {
		t.Fatalf("global log level = %s, want %s", got, zerolog.DebugLevel)
	}

	restore()
	if got := zerolog.GlobalLevel(); got != zerolog.InfoLevel {
		t.Fatalf("restored global log level = %s, want %s", got, zerolog.InfoLevel)
	}
}
