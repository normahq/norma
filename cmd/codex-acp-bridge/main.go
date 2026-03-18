package main

import (
	"os"

	codexacpbridge "github.com/metalagman/norma/internal/apps/codexacpbridge"
)

func main() {
	if err := codexacpbridge.Command().Execute(); err != nil {
		os.Exit(1)
	}
}
