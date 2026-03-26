package runscmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/normahq/norma/internal/config"
	"github.com/normahq/norma/internal/db"
	"github.com/spf13/viper"
)

func openDB(ctx context.Context) (*sql.DB, string, func(), error) {
	repoRoot, err := os.Getwd()
	if err != nil {
		return nil, "", func() {}, err
	}
	normaDir := filepath.Join(repoRoot, ".norma")
	if err := os.MkdirAll(normaDir, 0o700); err != nil {
		return nil, "", func() {}, err
	}
	dbPath := filepath.Join(normaDir, "norma.db")
	storeDB, err := db.Open(ctx, dbPath)
	if err != nil {
		return nil, "", func() {}, err
	}
	return storeDB, repoRoot, func() { _ = storeDB.Close() }, nil
}

func loadRuntimeAndCLIConfig(repoRoot string) (config.Config, config.CLISettings, error) {
	cfg, appCfg, err := config.LoadRuntimeAndCLIConfig(config.RuntimeLoadOptions{
		RepoRoot:  repoRoot,
		ConfigDir: viper.GetString("config_dir"),
		Profile:   viper.GetString("profile"),
	})
	if err != nil {
		return config.Config{}, config.CLISettings{}, err
	}
	if appCfg.EffectiveBudgets().MaxIterations <= 0 {
		return config.Config{}, config.CLISettings{}, fmt.Errorf("cli.budgets.max_iterations must be > 0")
	}
	return cfg, appCfg, nil
}
