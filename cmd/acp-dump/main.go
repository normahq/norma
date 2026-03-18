package main

import (
	"os"

	acpdump "github.com/metalagman/norma/internal/apps/acpdump"
)

func main() {
	if err := acpdump.Command().Execute(); err != nil {
		os.Exit(1)
	}
}
