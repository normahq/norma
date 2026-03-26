package config

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/normahq/norma/internal/adk/agentconfig"
)

// NormaConfig contains runtime agent-factory settings for Norma.
//
// Config shape:
//
//	norma:
//	  agents: ...
//	  mcp_servers: ...
type NormaConfig struct {
	Agents     map[string]agentconfig.Config          `json:"agents,omitempty"      mapstructure:"agents"      validate:"required,gt=0,dive,required"`
	MCPServers map[string]agentconfig.MCPServerConfig `json:"mcp_servers,omitempty" mapstructure:"mcp_servers" validate:"omitempty,dive,required"`
}

// Validate validates the runtime norma config.
func (c NormaConfig) Validate() error {
	errList := make([]string, 0)

	if err := normaConfigValidator.Struct(c); err != nil {
		if invErr, ok := err.(*validator.InvalidValidationError); ok {
			return fmt.Errorf("validate norma config: %w", invErr)
		}
		for _, validationErr := range err.(validator.ValidationErrors) {
			errList = append(errList, formatValidationError(validationErr))
		}
	}

	for name, agentCfg := range c.Agents {
		if err := agentCfg.Validate(); err != nil {
			errList = append(errList, fmt.Sprintf("agent %q: %v", name, err))
		}
	}

	for name, mcpCfg := range c.MCPServers {
		if err := agentconfig.ValidateMCPServerConfig(mcpCfg); err != nil {
			errList = append(errList, fmt.Sprintf("mcp %q: %v", name, err))
		}
	}

	if len(errList) == 0 {
		return nil
	}
	sort.Strings(errList)
	return fmt.Errorf("norma config validation failed: %s", strings.Join(errList, "; "))
}

var normaConfigValidator = newNormaConfigValidator()

func newNormaConfigValidator() *validator.Validate {
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
