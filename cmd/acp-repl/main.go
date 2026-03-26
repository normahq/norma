package main

import (
	"os"

	"github.com/normahq/norma/cmd/acp-repl/cmd"
)

func main() {
	if err := command.Command().Execute(); err != nil {
		os.Exit(1)
	}
}
