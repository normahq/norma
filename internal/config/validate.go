package config

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/normahq/norma/internal/adk/agentconfig"
)

var configValidator = newConfigValidator()

func newConfigValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("mapstructure"), ",", 2)[0]
		if name == "" || name == "-" {
			return fld.Name
		}
		return name
	})
	return v
}

// ValidateSettings validates raw config settings against the Config struct tags and custom logic.
func ValidateSettings(settings map[string]any) error {
	var cfg Config
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Metadata:    nil,
		Result:      &cfg,
		TagName:     "mapstructure",
		ErrorUnused: false,
	})
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}
	if err := decoder.Decode(settings); err != nil {
		return fmt.Errorf("failed to decode settings: %w", err)
	}

	return cfg.Validate()
}

// Validate validates the configuration.
func (c Config) Validate() error {
	errs := make([]string, 0)

	// 1. Structural validation via tags
	if err := configValidator.Struct(c); err != nil {
		if _, ok := err.(*validator.InvalidValidationError); ok {
			return fmt.Errorf("validate config: %w", err)
		}
		for _, validationErr := range err.(validator.ValidationErrors) {
			errs = append(errs, formatValidationError(validationErr))
		}
	}

	// 2. Custom validation for Agents
	for name, agentCfg := range c.Agents {
		if err := agentCfg.Validate(); err != nil {
			errs = append(errs, fmt.Sprintf("agent %q: %v", name, err))
		}
	}

	// 3. Custom validation for MCPServers
	for name, mcpCfg := range c.MCPServers {
		if err := agentconfig.ValidateMCPServerConfig(mcpCfg); err != nil {
			errs = append(errs, fmt.Sprintf("mcp_server %q: %v", name, err))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)

	return fmt.Errorf("config validation failed: %s", strings.Join(errs, "; "))
}

func formatValidationError(err validator.FieldError) string {
	field := err.Field()
	switch err.Tag() {
	case "required":
		return field + " is required"
	case "gt":
		return field + " must be greater than " + err.Param()
	case "min":
		return field + " must be at least " + err.Param()
	default:
		return field + " failed validation rule " + err.Tag()
	}
}
