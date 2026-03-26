package plancmd

import (
	"github.com/normahq/norma/internal/config"
	"github.com/spf13/viper"
)

func loadConfig(repoRoot string) (config.Config, error) {
	return config.LoadRuntime(config.RuntimeLoadOptions{
		RepoRoot:  repoRoot,
		ConfigDir: viper.GetString("config_dir"),
		Profile:   viper.GetString("profile"),
	})
}
