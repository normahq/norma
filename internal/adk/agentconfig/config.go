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
	Type       MCPServerType     `json:"type"                  mapstructure:"type"        validate:"required"`
	Cmd        []string          `json:"cmd,omitempty"         mapstructure:"cmd"`
	Args       []string          `json:"args,omitempty"        mapstructure:"args"`
	Env        map[string]string `json:"env,omitempty"         mapstructure:"env"`
	WorkingDir string            `json:"working_dir,omitempty" mapstructure:"working_dir"`
	URL        string            `json:"url,omitempty"         mapstructure:"url"`
	Headers    map[string]string `json:"headers,omitempty"     mapstructure:"headers"`
}

// ACPConfig is an ACP runtime configuration block used by generic and alias types.
type ACPConfig struct {
	Cmd       []string `json:"cmd,omitempty"       mapstructure:"cmd"`
	ExtraArgs []string `json:"extra_args,omitempty" mapstructure:"extra_args"`
	Model     string   `json:"model,omitempty"     mapstructure:"model"     validate:"omitempty,min=1"`
	Mode      string   `json:"mode,omitempty"      mapstructure:"mode"      validate:"omitempty,min=1"`
}

// PoolConfig is the pool runtime configuration block.
type PoolConfig struct {
	Members []string `json:"members,omitempty" mapstructure:"members"`
}

// Config describes how to run an agent.
//
// The schema is strict and discriminated by type:
//
//	type: <agent_type>
//	<agent_type>:
//	  ...type-specific config...
//
// The fields Cmd/Model/.../Pool are derived runtime fields populated by
// normalization for backwards-compatible runtime call sites.
type Config struct {
	Type              string   `json:"type"                           mapstructure:"type"               validate:"required"`
	MCPServers        []string `json:"mcp_servers,omitempty"          mapstructure:"mcp_servers"`
	SystemInstruction string   `json:"system_instruction,omitempty"    mapstructure:"system_instruction" validate:"omitempty,min=1"`

	GenericACP  *ACPConfig  `json:"generic_acp,omitempty"  mapstructure:"generic_acp"`
	GeminiACP   *ACPConfig  `json:"gemini_acp,omitempty"   mapstructure:"gemini_acp"`
	CodexACP    *ACPConfig  `json:"codex_acp,omitempty"    mapstructure:"codex_acp"`
	OpenCodeACP *ACPConfig  `json:"opencode_acp,omitempty" mapstructure:"opencode_acp"`
	CopilotACP  *ACPConfig  `json:"copilot_acp,omitempty"  mapstructure:"copilot_acp"`
	PoolConfig  *PoolConfig `json:"pool,omitempty"         mapstructure:"pool"`

	// Derived runtime fields populated by NormalizeACPConfig/NormalizeACPConfigs.
	Cmd       []string `json:"cmd,omitempty"       mapstructure:"-"`
	ExtraArgs []string `json:"extra_args,omitempty" mapstructure:"-"`
	Model     string   `json:"model,omitempty"     mapstructure:"-"`
	Mode      string   `json:"mode,omitempty"      mapstructure:"-"`
	Pool      []string `json:"pool_members,omitempty" mapstructure:"-"`
}

// Description returns a human-readable description of the agent config.
// Format: "name: type=<type> model=<model> mode=<mode>" with missing parts omitted.
func (c Config) Description(name string) string {
	var parts []string
	if c.Type != "" {
		parts = append(parts, fmt.Sprintf("type=%s", c.Type))
	}
	model := c.Model
	if model == "" {
		if spec, ok := c.selectedACPBlock(); ok {
			model = spec.Model
		}
	}
	if model != "" {
		parts = append(parts, fmt.Sprintf("model=%s", model))
	}
	mode := c.Mode
	if mode == "" {
		if spec, ok := c.selectedACPBlock(); ok {
			mode = spec.Mode
		}
	}
	if mode != "" {
		parts = append(parts, fmt.Sprintf("mode=%s", mode))
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

	for i, serverID := range c.MCPServers {
		if strings.TrimSpace(serverID) == "" {
			errs = append(errs, fmt.Sprintf("mcp_servers[%d] must have at least 1 character", i))
		}
	}

	typeBlocks := map[string]bool{
		AgentTypeGenericACP:  c.GenericACP != nil,
		AgentTypeGeminiACP:   c.GeminiACP != nil,
		AgentTypeCodexACP:    c.CodexACP != nil,
		AgentTypeOpenCodeACP: c.OpenCodeACP != nil,
		AgentTypeCopilotACP:  c.CopilotACP != nil,
		AgentTypePool:        c.PoolConfig != nil,
	}
	selectedCount := 0
	for _, present := range typeBlocks {
		if present {
			selectedCount++
		}
	}
	if selectedCount != 1 {
		errs = append(errs, "exactly one type-specific block must be set")
	}
	if c.Type != "" {
		if present, ok := typeBlocks[c.Type]; ok && !present {
			errs = append(errs, fmt.Sprintf("%s block is required for type %s", c.Type, c.Type))
		}
		for typeName, present := range typeBlocks {
			if !present || typeName == c.Type {
				continue
			}
			errs = append(errs, fmt.Sprintf("%s block must be omitted when type is %s", typeName, c.Type))
		}
	}

	switch strings.TrimSpace(c.Type) {
	case AgentTypeGenericACP:
		errs = append(errs, validateACPBlock(c.GenericACP, true, AgentTypeGenericACP)...)
	case AgentTypeGeminiACP:
		errs = append(errs, validateACPBlock(c.GeminiACP, false, AgentTypeGeminiACP)...)
	case AgentTypeCodexACP:
		errs = append(errs, validateACPBlock(c.CodexACP, false, AgentTypeCodexACP)...)
	case AgentTypeOpenCodeACP:
		errs = append(errs, validateACPBlock(c.OpenCodeACP, false, AgentTypeOpenCodeACP)...)
	case AgentTypeCopilotACP:
		errs = append(errs, validateACPBlock(c.CopilotACP, false, AgentTypeCopilotACP)...)
	case AgentTypePool:
		errs = append(errs, validatePoolBlock(c.PoolConfig)...)
	}

	if len(errs) == 0 {
		return nil
	}
	sort.Strings(errs)

	return fmt.Errorf("agent config schema validation failed: %s", strings.Join(errs, "; "))
}

func validateACPBlock(block *ACPConfig, cmdRequired bool, typeName string) []string {
	errs := make([]string, 0)
	if block == nil {
		return errs
	}
	if err := configValidator.Struct(block); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			for _, validationErr := range validationErrs {
				errs = append(errs, formatValidationError(validationErr))
			}
		}
	}
	if cmdRequired {
		if len(block.Cmd) == 0 {
			errs = append(errs, fmt.Sprintf("cmd is required for type %s", typeName))
		}
	} else if len(block.Cmd) > 0 {
		errs = append(errs, fmt.Sprintf("cmd must be omitted for type %s", typeName))
	}
	for i, arg := range block.Cmd {
		if strings.TrimSpace(arg) == "" {
			errs = append(errs, fmt.Sprintf("cmd[%d] must have at least 1 character", i))
		}
	}
	for i, arg := range block.ExtraArgs {
		if strings.TrimSpace(arg) == "" {
			errs = append(errs, fmt.Sprintf("extra_args[%d] must have at least 1 character", i))
		}
	}
	return errs
}

func validatePoolBlock(block *PoolConfig) []string {
	errs := make([]string, 0)
	if block == nil {
		return errs
	}
	if len(block.Members) == 0 {
		errs = append(errs, "pool.members is required for type pool")
	}
	for i, member := range block.Members {
		if strings.TrimSpace(member) == "" {
			errs = append(errs, fmt.Sprintf("pool.members[%d] must have at least 1 character", i))
		}
	}
	return errs
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

// NormalizeACPConfig canonicalizes ACP aliases to generic_acp while preserving behavior.
func NormalizeACPConfig(cfg Config, executablePath string) (Config, error) {
	normalized := cfg
	normalized.Cmd = nil
	normalized.ExtraArgs = nil
	normalized.Model = ""
	normalized.Mode = ""
	normalized.Pool = nil

	switch strings.TrimSpace(cfg.Type) {
	case AgentTypeGeminiACP:
		if cfg.GeminiACP == nil {
			return Config{}, fmt.Errorf("gemini_acp block is required")
		}
		normalized.Type = AgentTypeGenericACP
		specCopy := *cfg.GeminiACP
		specCopy.Cmd = []string{"gemini", "--acp"}
		if specCopy.Model != "" {
			specCopy.Cmd = append(specCopy.Cmd, "--model", specCopy.Model)
		}
		normalized.GenericACP = &specCopy
		normalized.GeminiACP = nil
	case AgentTypeOpenCodeACP:
		if cfg.OpenCodeACP == nil {
			return Config{}, fmt.Errorf("opencode_acp block is required")
		}
		normalized.Type = AgentTypeGenericACP
		specCopy := *cfg.OpenCodeACP
		specCopy.Cmd = []string{"opencode", "acp"}
		normalized.GenericACP = &specCopy
		normalized.OpenCodeACP = nil
	case AgentTypeCodexACP:
		if cfg.CodexACP == nil {
			return Config{}, fmt.Errorf("codex_acp block is required")
		}
		exePath := strings.TrimSpace(executablePath)
		if exePath == "" {
			return Config{}, fmt.Errorf("resolve executable path: empty")
		}
		normalized.Type = AgentTypeGenericACP
		specCopy := *cfg.CodexACP
		specCopy.Cmd = []string{exePath, "tool", "codex-acp-bridge"}
		if specCopy.Model != "" {
			specCopy.Cmd = append(specCopy.Cmd, "--codex-model", specCopy.Model)
		}
		normalized.GenericACP = &specCopy
		normalized.CodexACP = nil
	case AgentTypeCopilotACP:
		if cfg.CopilotACP == nil {
			return Config{}, fmt.Errorf("copilot_acp block is required")
		}
		normalized.Type = AgentTypeGenericACP
		specCopy := *cfg.CopilotACP
		specCopy.Cmd = []string{"copilot", "--acp"}
		normalized.GenericACP = &specCopy
		normalized.CopilotACP = nil
	case AgentTypeGenericACP:
		if cfg.GenericACP == nil {
			return Config{}, fmt.Errorf("generic_acp block is required")
		}
	case AgentTypePool:
		if cfg.PoolConfig == nil {
			return Config{}, fmt.Errorf("pool block is required")
		}
	}

	normalized.populateRuntimeFields()
	return normalized, nil
}

func (c *Config) populateRuntimeFields() {
	if c == nil {
		return
	}
	if c.Type == AgentTypePool {
		if c.PoolConfig != nil {
			c.Pool = append([]string(nil), c.PoolConfig.Members...)
		}
		return
	}
	spec, ok := c.selectedACPBlock()
	if !ok || spec == nil {
		return
	}
	c.Cmd = append([]string(nil), spec.Cmd...)
	c.ExtraArgs = append([]string(nil), spec.ExtraArgs...)
	c.Model = spec.Model
	c.Mode = spec.Mode
}

func (c Config) selectedACPBlock() (*ACPConfig, bool) {
	switch strings.TrimSpace(c.Type) {
	case AgentTypeGenericACP:
		return c.GenericACP, c.GenericACP != nil
	case AgentTypeGeminiACP:
		return c.GeminiACP, c.GeminiACP != nil
	case AgentTypeCodexACP:
		return c.CodexACP, c.CodexACP != nil
	case AgentTypeOpenCodeACP:
		return c.OpenCodeACP, c.OpenCodeACP != nil
	case AgentTypeCopilotACP:
		return c.CopilotACP, c.CopilotACP != nil
	default:
		return nil, false
	}
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
