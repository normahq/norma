package acpcmd

import (
	"strings"
	"testing"
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
