package main

import (
	"path/filepath"
	"testing"

	initcmd "github.com/normahq/norma/cmd/norma/init"
	"github.com/spf13/viper"
)

func TestDefaultConfigYAML_IsLoadable(t *testing.T) {
	workingDir := t.TempDir()
	t.Setenv("GOOGLE_API_KEY", "test-google-api-key")
	if err := writeTestFile(filepath.Join(workingDir, defaultConfigPath), initcmd.DefaultConfigYAML); err != nil {
		t.Fatalf("write default config: %v", err)
	}

	viper.Reset()
	t.Cleanup(viper.Reset)

	if _, err := loadConfig(workingDir); err != nil {
		t.Fatalf("load default config: %v", err)
	}
}
