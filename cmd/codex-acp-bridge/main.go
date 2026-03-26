package main

import (
	"os"

	"github.com/normahq/norma/cmd/codex-acp-bridge/cmd"
)

func main() {
	if err := command.Command().Execute(); err != nil {
		os.Exit(1)
	}
}
