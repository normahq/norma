package main

import (
	"path/filepath"

	"github.com/normahq/norma/internal/config"
	"github.com/spf13/viper"
)

var defaultConfigPath = filepath.Join(".norma", config.CoreConfigFileName)

func loadConfig(repoRoot string) (config.Config, error) {
	return config.LoadRuntime(config.RuntimeLoadOptions{
		RepoRoot:  repoRoot,
		ConfigDir: viper.GetString("config_dir"),
		Profile:   viper.GetString("profile"),
	})
}

func loadRuntimeAndCLIConfig(repoRoot string) (config.Config, config.CLISettings, error) {
	return config.LoadRuntimeAndCLIConfig(config.RuntimeLoadOptions{
		RepoRoot:  repoRoot,
		ConfigDir: viper.GetString("config_dir"),
		Profile:   viper.GetString("profile"),
	})
}
