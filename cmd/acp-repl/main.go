package main

import (
	"os"

	acprepl "github.com/metalagman/norma/internal/apps/acprepl"
)

func main() {
	if err := acprepl.Command().Execute(); err != nil {
		os.Exit(1)
	}
}
