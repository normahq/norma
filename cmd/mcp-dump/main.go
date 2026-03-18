package main

import (
	"os"

	mcpdump "github.com/metalagman/norma/internal/apps/mcpdump"
)

func main() {
	if err := mcpdump.Command().Execute(); err != nil {
		os.Exit(1)
	}
}
