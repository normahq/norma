package main

import (
	"os"

	"github.com/normahq/norma/cmd/mcp-dump/cmd"
)

func main() {
	if err := command.Command().Execute(); err != nil {
		os.Exit(1)
	}
}
