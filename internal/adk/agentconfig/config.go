package agentconfig

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
)

// MCPServerType represents the transport type for an MCP server.
type MCPServerType string

const (
	// MCPServerTypeStdio is the stdio transport type.
	MCPServerTypeStdio MCPServerType = "stdio"
	// MCPServerTypeHTTP is the HTTP transport type.
	MCPServerTypeHTTP MCPServerType = "http"
	// MCPServerTypeSSE is the SSE (Server-Sent Events) transport type.
	MCPServerTypeSSE MCPServerType = "sse"
)

// MCPServerConfig describes how to connect to an MCP server.
type MCPServerConfig struct {
	Type       MCPServerType     `json:"type"                 mapstructure:"type"        validate:"required"`
	Cmd        []string          `json:"cmd,omitempty"       mapstructure:"cmd"`
	Args       []string          `json:"args,omitempty"      mapstructure:"args"`
	Env        map[string]string `json:"env,omitempty"       mapstructure:"env"`
	WorkingDir string            `json:"working_dir,omitempty" mapstructure:"working_dir"`
	URL        string            `json:"url,omitempty"       mapstructure:"url"`
	Headers    map[string]string `json:"headers,omitempty"   mapstructure:"headers"`
}

// Config describes how to run an agent.
type Config struct {
	Type              string   `json:"type"                   mapstructure:"type"               validate:"required"`
	Cmd               []string `json:"cmd,omitempty"          mapstructure:"cmd"`
	ExtraArgs         []string `json:"extra_args,omitempty"   mapstructure:"extra_args"`
	Model             string   `json:"model,omitempty"        mapstructure:"model"              validate:"omitempty,min=1"`
	Mode              string   `json:"mode,omitempty"         mapstructure:"mode"               validate:"omitempty,min=1"`
	BaseURL           string   `json:"base_url,omitempty"     mapstructure:"base_url"           validate:"omitempty,min=1"`
	APIKey            string   `json:"api_key,omitempty"      mapstructure:"api_key"            validate:"omitempty,min=1"`
	Timeout           int      `json:"timeout,omitempty"      mapstructure:"timeout"            validate:"omitempty,min=1"`
	UseTTY            *bool    `json:"use_tty,omitempty"      mapstructure:"use_tty"`
	Pool              []string `json:"pool,omitempty"         mapstructure:"pool"`
	MCPServers        any      `json:"mcp_servers,omitempty"  mapstructure:"mcp_servers"`
	SystemInstruction string   `json:"system_instruction,omitempty" mapstructure:"system_instruction" validate:"omitempty,min=1"`
}

// Description returns a human-readable description of the agent config.
// Format: "name: type=<type> model=<model> mode=<mode>" with missing parts omitted.
func (c Config) Description(name string) string {
	var parts []string
	if c.Type != "" {
		parts = append(parts, fmt.Sprintf("type=%s", c.Type))
	}
	if c.Model != "" {
		parts = append(parts, fmt.Sprintf("model=%s", c.Model))
	}
	if c.Mode != "" {
		parts = append(parts, fmt.Sprintf("mode=%s", c.Mode))
	}
	if len(parts) == 0 {
		return name
	}
	return fmt.Sprintf("%s: %s", name, strings.Join(parts, " "))
}

var configValidator = newConfigValidator()

func newConfigValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return fld.Name
		}
		return name
	})
	return v
}

// Validate validates the agent configuration.
func (c Config) Validate() error {
	errs := make([]string, 0)

	if err := configValidator.Struct(c); err != nil {
		if invErr, ok := err.(*validator.InvalidValidationError); ok {
			return fmt.Errorf("validate agent config: %w", invErr)
		}
		for _, validationErr := range err.(validator.ValidationErrors) {
			errs = append(errs, formatValidationError(validationErr))
		}
	}

	if !IsValidAgentType(c.Type) {
		errs = append(errs, fmt.Sprintf("type must be one of: %s", strings.Join(SupportedAgentTypes(), ", ")))
	}

	switch c.Type {
	case AgentTypeGenericACP:
		if len(c.Cmd) == 0 {
			errs = append(errs, fmt.Sprintf("cmd is required for type %s", c.Type))
		}
	case AgentTypeCodexACP, AgentTypeOpenCodeACP, AgentTypeGeminiACP, AgentTypeCopilotACP:
		if len(c.Cmd) > 0 {
			errs = append(errs, fmt.Sprintf("cmd must be omitted for type %s", c.Type))
		}
	case AgentTypePool:
		if len(c.Pool) == 0 {
			errs = append(errs, "pool is required for type pool")
		}
	}

	for i, arg := range c.Cmd {
		if arg == "" {
			errs = append(errs, fmt.Sprintf("cmd[%d] must have at least 1 character", i))
		}
	}
	for i, arg := range c.ExtraArgs {
		if arg == "" {
			errs = append(errs, fmt.Sprintf("extra_args[%d] must have at least 1 character", i))
		}
	}
	for i, member := range c.Pool {
		if strings.TrimSpace(member) == "" {
			errs = append(errs, fmt.Sprintf("pool[%d] must have at least 1 character", i))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)

	return fmt.Errorf("agent config schema validation failed: %s", strings.Join(errs, "; "))
}

func formatValidationError(err validator.FieldError) string {
	field := err.Field()
	switch err.Tag() {
	case "required":
		return field + " is required"
	case "oneof":
		return field + " must be one of: " + err.Param()
	case "min":
		return field + " must be at least " + err.Param()
	default:
		return field + " failed validation rule " + err.Tag()
	}
}

const (
	// AgentTypeGenericACP is the type for custom ACP CLI executables.
	AgentTypeGenericACP = "generic_acp"

	// AgentTypeGeminiACP is the alias for Gemini CLI ACP mode.
	AgentTypeGeminiACP = "gemini_acp"
	// AgentTypeCodexACP is the alias for Codex ACP bridge mode.
	AgentTypeCodexACP = "codex_acp"
	// AgentTypeOpenCodeACP is the alias for OpenCode CLI ACP mode.
	AgentTypeOpenCodeACP = "opencode_acp"
	// AgentTypeCopilotACP is the alias for Copilot CLI ACP mode.
	AgentTypeCopilotACP = "copilot_acp"
	// AgentTypePool is the pool type with ordered failover.
	AgentTypePool = "pool"
)

// SupportedAgentTypes returns all supported agent types.
func SupportedAgentTypes() []string {
	return []string{
		AgentTypeGenericACP,
		AgentTypeGeminiACP,
		AgentTypeCodexACP,
		AgentTypeOpenCodeACP,
		AgentTypeCopilotACP,
		AgentTypePool,
	}
}

// IsValidAgentType reports whether the given type is a valid agent type.
func IsValidAgentType(agentType string) bool {
	for _, t := range SupportedAgentTypes() {
		if t == agentType {
			return true
		}
	}
	return false
}

// IsPoolType reports whether an agent type is a pool.
func IsPoolType(agentType string) bool {
	return strings.TrimSpace(agentType) == AgentTypePool
}

// IsACPType reports whether an agent type uses the ACP runtime.
func IsACPType(agentType string) bool {
	switch strings.TrimSpace(agentType) {
	case AgentTypeGenericACP, AgentTypeGeminiACP, AgentTypeOpenCodeACP, AgentTypeCodexACP, AgentTypeCopilotACP:
		return true
	default:
		return false
	}
}

// IsPlannerSupportedType reports whether planner mode supports the agent type.
func IsPlannerSupportedType(agentType string) bool {
	return IsACPType(agentType)
}

// SupportedMCPServerTypes returns all supported MCP server types.
func SupportedMCPServerTypes() []MCPServerType {
	return []MCPServerType{
		MCPServerTypeStdio,
		MCPServerTypeHTTP,
		MCPServerTypeSSE,
	}
}

// IsValidMCPServerType reports whether the given type is a valid MCP server type.
func IsValidMCPServerType(serverType MCPServerType) bool {
	for _, t := range SupportedMCPServerTypes() {
		if t == serverType {
			return true
		}
	}
	return false
}

// ValidateMCPServerConfig validates an MCP server configuration.
func ValidateMCPServerConfig(cfg MCPServerConfig) error {
	errs := make([]string, 0)

	if !IsValidMCPServerType(cfg.Type) {
		errs = append(errs, fmt.Sprintf("type must be one of: %v", SupportedMCPServerTypes()))
	}

	switch cfg.Type {
	case MCPServerTypeStdio:
		if len(cfg.Cmd) == 0 {
			errs = append(errs, "cmd is required for stdio type")
		}
	case MCPServerTypeHTTP, MCPServerTypeSSE:
		if strings.TrimSpace(cfg.URL) == "" {
			errs = append(errs, "url is required for http/sse type")
		}
	}

	for i, arg := range cfg.Cmd {
		if arg == "" {
			errs = append(errs, fmt.Sprintf("cmd[%d] must have at least 1 character", i))
		}
	}
	for i, arg := range cfg.Args {
		if arg == "" {
			errs = append(errs, fmt.Sprintf("args[%d] must have at least 1 character", i))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)
	return fmt.Errorf("mcp server config validation failed: %s", strings.Join(errs, "; "))
}

// MCPServerNames returns the list of MCP server names from an agent's MCPServers field.
// It handles both single string and array of strings formats.
func MCPServerNames(mcpServers any) ([]string, error) {
	if mcpServers == nil {
		return nil, nil
	}

	switch v := mcpServers.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("mcp_servers string value cannot be empty")
		}
		return []string{strings.TrimSpace(v)}, nil
	case []any:
		names := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("mcp_servers[%d] must be a String", i)
			}
			if strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("mcp_servers[%d] cannot be empty", i)
			}
			names = append(names, strings.TrimSpace(s))
		}
		return names, nil
	default:
		return nil, fmt.Errorf("mcp_servers must be a string or array of strings, got %T", mcpServers)
	}
}

// NormalizeACPConfig canonicalizes ACP aliases to generic_acp while preserving behavior.
func NormalizeACPConfig(cfg Config, executablePath string) (Config, error) {
	normalized := cfg

	switch strings.TrimSpace(cfg.Type) {
	case AgentTypeGeminiACP:
		normalized.Type = AgentTypeGenericACP
		normalized.Cmd = []string{"gemini", "--experimental-acp"}
		if cfg.Model != "" {
			normalized.Cmd = append(normalized.Cmd, "--model", cfg.Model)
		}
	case AgentTypeOpenCodeACP:
		normalized.Type = AgentTypeGenericACP
		normalized.Cmd = []string{"opencode", "acp"}
	case AgentTypeCodexACP:
		exePath := strings.TrimSpace(executablePath)
		if exePath == "" {
			return Config{}, fmt.Errorf("resolve executable path: empty")
		}
		normalized.Type = AgentTypeGenericACP
		normalized.Cmd = []string{exePath, "tool", "codex-acp-bridge"}
		if cfg.Model != "" {
			normalized.Cmd = append(normalized.Cmd, "--codex-model", cfg.Model)
		}
	case AgentTypeCopilotACP:
		normalized.Type = AgentTypeGenericACP
		normalized.Cmd = []string{"copilot", "--acp"}
	}

	return normalized, nil
}

// NormalizeACPConfigs canonicalizes ACP aliases for a map of named agent configs.
func NormalizeACPConfigs(cfgs map[string]Config, executablePath string) (map[string]Config, error) {
	if len(cfgs) == 0 {
		return cfgs, nil
	}

	normalized := make(map[string]Config, len(cfgs))
	for name, cfg := range cfgs {
		normCfg, err := NormalizeACPConfig(cfg, executablePath)
		if err != nil {
			return nil, fmt.Errorf("normalize agent %q: %w", name, err)
		}
		normalized[name] = normCfg
	}

	return normalized, nil
}
