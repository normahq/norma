package relaycmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestConfigLoading(t *testing.T) {
	// Setup: create a temporary directory to act as repo root
	tmpDir, err := os.MkdirTemp("", "norma-test-*")
	assert.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("failed to remove temp dir: %v", err)
		}
	}()

	normaDir := filepath.Join(tmpDir, ".norma")
	err = os.Mkdir(normaDir, 0755)
	assert.NoError(t, err)

	// Create shared config
	configContent := "profiles:\n  default:\n    relay: opencode\n"
	err = os.WriteFile(filepath.Join(normaDir, "config.yaml"), []byte(configContent), 0644)
	assert.NoError(t, err)

	// Prepare Viper (shared config)
	viper.SetConfigFile(filepath.Join(normaDir, "config.yaml"))
	err = viper.ReadInConfig()
	assert.NoError(t, err)

	// Test: Run initConfig
	err = initConfig(tmpDir)
	assert.NoError(t, err)

	// Check if merged successfully
	assert.Equal(t, "opencode", viper.GetString("profiles.default.relay"))
	assert.Equal(t, "info", viper.GetString("relay.logger.level")) // From embedded relay.yaml
}
