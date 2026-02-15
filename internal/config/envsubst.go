// Package config provides configuration loading and management for norma.
package config

import (
	"os"
)

// ExpandEnv expands $VAR and ${VAR} placeholders in the provided text.
func ExpandEnv(input string) (string, error) {
	return os.ExpandEnv(input), nil
}
