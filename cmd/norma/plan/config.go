package plancmd

import (
	"github.com/normahq/norma/internal/config"
	"github.com/spf13/viper"
)

func loadConfig(workingDir string) (config.Config, error) {
	return config.LoadRuntime(config.RuntimeLoadOptions{
		WorkingDir: workingDir,
		ConfigDir:  viper.GetString("config_dir"),
		Profile:    viper.GetString("profile"),
	})
}
