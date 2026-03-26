// Package agentfactory provides a registry and factory for creating ADK-compatible agents.
package agentfactory

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/coder/acp-go-sdk"
	"github.com/normahq/norma/internal/adk/acpagent"
	"github.com/normahq/norma/internal/adk/agentconfig"
	"github.com/normahq/norma/internal/adk/poolagent"
	"github.com/rs/zerolog"

	"google.golang.org/adk/agent"
)

// CreationRequest defines the parameters for creating a new agent.
type CreationRequest struct {
	Name              string
	Description       string
	SystemInstruction string
	WorkingDirectory  string
	Stdout            io.Writer
	Stderr            io.Writer
	Logger            *zerolog.Logger
	PermissionHandler func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error)
	MCPServers        map[string]agentconfig.MCPServerConfig
}

// constructor is a function that creates a new agent instance.
type constructor func(ctx context.Context, cfg agentconfig.Config, req CreationRequest, registry map[string]agentconfig.Config) (agent.Agent, error)

// Factory is a registry of agent configurations.
type Factory struct {
	mu         sync.RWMutex
	registry   map[string]agentconfig.Config
	mcpServers map[string]agentconfig.MCPServerConfig
}

// NewFactory creates a new Factory from a map of agent configurations.
func NewFactory(agents map[string]agentconfig.Config) *Factory {
	return &Factory{
		registry: agents,
	}
}

// NewFactoryWithMCPServers creates a new Factory from a map of agent configurations and MCP servers.
func NewFactoryWithMCPServers(agents map[string]agentconfig.Config, mcpServers map[string]agentconfig.MCPServerConfig) *Factory {
	return &Factory{
		registry:   agents,
		mcpServers: mcpServers,
	}
}

// AddMCPServer adds an MCP server configuration to the factory.
func (f *Factory) AddMCPServer(name string, cfg agentconfig.MCPServerConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mcpServers == nil {
		f.mcpServers = make(map[string]agentconfig.MCPServerConfig)
	}
	f.mcpServers[name] = cfg
}

// GetMCPServer returns the MCP server configuration for the given name.
func (f *Factory) GetMCPServer(name string) (agentconfig.MCPServerConfig, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	cfg, ok := f.mcpServers[name]
	return cfg, ok
}

// GetAgentConfig returns the agent configuration for the given name.
// It returns an error if the agent is not found.
func (f *Factory) GetAgentConfig(name string) (agentconfig.Config, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	cfg, ok := f.registry[name]
	if !ok {
		return agentconfig.Config{}, fmt.Errorf("agent %q not found in registry", name)
	}
	return cfg, nil
}

// ValidateAgent checks if an agent with the given name can be created.
// It returns an error if the agent is not found or its type is unsupported.
func (f *Factory) ValidateAgent(name string) error {
	cfg, err := f.GetAgentConfig(name)
	if err != nil {
		return err
	}
	// Check if the type is supported
	if _, ok := constructors[cfg.Type]; !ok {
		return fmt.Errorf("agent type %q is not supported", cfg.Type)
	}
	return nil
}

// CreateAgent creates an agent.Agent instance by name and creation request.
// It returns an error if the agent is not found or its type is unsupported.
func (f *Factory) CreateAgent(ctx context.Context, name string, req CreationRequest) (agent.Agent, error) {
	if strings.TrimSpace(req.WorkingDirectory) == "" {
		return nil, fmt.Errorf("working directory is required")
	}

	f.mu.RLock()
	cfg, ok := f.registry[name]
	mcpServers := f.mcpServers
	f.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent %q not found or unsupported", name)
	}

	if req.MCPServers == nil {
		req.MCPServers = mcpServers
	}

	create, ok := constructors[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported agent type %q for agent %q", cfg.Type, name)
	}

	ag, err := create(ctx, cfg, req, f.registry)
	if err != nil {
		return nil, fmt.Errorf("create agent %q: %w", name, err)
	}

	return ag, nil
}

// constructors registry.
var constructors = map[string]constructor{
	agentconfig.AgentTypeGenericACP: acpConstructor,
	agentconfig.AgentTypePool:       poolConstructor,
}

var newACPAgent = func(cfg acpagent.Config) (agent.Agent, error) {
	return acpagent.New(cfg)
}

var acpConstructor = func(ctx context.Context, cfg agentconfig.Config, req CreationRequest, _ map[string]agentconfig.Config) (agent.Agent, error) {
	cmd, err := ResolveACPCommand(cfg)
	if err != nil {
		return nil, err
	}
	loggerCtx := ctx
	if loggerCtx == nil {
		loggerCtx = context.Background()
	}
	logger := req.Logger
	if logger == nil {
		logger = zerolog.Ctx(loggerCtx)
	}

	return newACPAgent(acpagent.Config{
		Context:            ctx,
		Name:               req.Name,
		Description:        req.Description,
		Model:              cfg.Model,
		Mode:               cfg.Mode,
		SystemInstructions: req.SystemInstruction,
		Command:            cmd,
		WorkingDir:         req.WorkingDirectory,
		Stderr:             req.Stderr,
		PermissionHandler:  req.PermissionHandler,
		Logger:             logger,
		MCPServers:         req.MCPServers,
	})
}

var poolConstructor = func(ctx context.Context, cfg agentconfig.Config, req CreationRequest, registry map[string]agentconfig.Config) (agent.Agent, error) {
	members, err := validatePoolMembers(req.Name, cfg.Pool, registry)
	if err != nil {
		return nil, err
	}

	poolMembers := make([]poolagent.MemberConfig, len(members))
	for i, m := range members {
		poolMembers[i] = poolagent.MemberConfig{
			Name: m.Name,
			Cfg:  m.Cfg,
		}
	}

	poolReq := poolagent.AgentRequest{
		Name:              req.Name,
		Description:       req.Description,
		SystemInstruction: req.SystemInstruction,
		WorkingDirectory:  req.WorkingDirectory,
	}

	creator := &factoryAgentCreator{registry: registry, req: req}

	return poolagent.NewPoolAgent(ctx, req.Name, poolMembers, poolReq, creator)
}

type factoryAgentCreator struct {
	registry map[string]agentconfig.Config
	req      CreationRequest
}

func (f *factoryAgentCreator) CreateAgent(ctx context.Context, name string, req poolagent.AgentRequest) (agent.Agent, error) {
	fullReq := CreationRequest{
		Name:              req.Name,
		Description:       req.Description,
		SystemInstruction: req.SystemInstruction,
		WorkingDirectory:  req.WorkingDirectory,
		Stderr:            f.req.Stderr,
		Logger:            f.req.Logger,
		MCPServers:        f.req.MCPServers,
	}
	return NewFactoryWithMCPServers(f.registry, f.req.MCPServers).CreateAgent(ctx, name, fullReq)
}

type poolMemberConfig struct {
	Name string
	Cfg  agentconfig.Config
}

func validatePoolMembers(poolName string, pool []string, registry map[string]agentconfig.Config) ([]poolMemberConfig, error) {
	if len(pool) == 0 {
		return nil, fmt.Errorf("pool agent requires pool members")
	}

	members := make([]poolMemberConfig, 0, len(pool))
	for i, memberName := range pool {
		memberName = strings.TrimSpace(memberName)
		if memberName == "" {
			return nil, fmt.Errorf("pool member at index %d is empty", i)
		}
		if memberName == poolName {
			return nil, fmt.Errorf("pool cannot reference itself")
		}
		memberCfg, ok := registry[memberName]
		if !ok {
			return nil, fmt.Errorf("pool references unknown agent %q", memberName)
		}
		if agentconfig.IsPoolType(memberCfg.Type) {
			return nil, fmt.Errorf("pool cannot contain nested pool %q", memberName)
		}
		members = append(members, poolMemberConfig{
			Name: memberName,
			Cfg:  memberCfg,
		})
	}
	return members, nil
}

// ResolveACPCommand resolves the command for ACP-backed agent types.
func ResolveACPCommand(cfg agentconfig.Config) ([]string, error) {
	if cfg.Type != agentconfig.AgentTypeGenericACP {
		return nil, fmt.Errorf("unknown acp agent type %q", cfg.Type)
	}

	if len(cfg.Cmd) == 0 {
		return nil, fmt.Errorf("generic_acp agent requires cmd")
	}
	cmd := cfg.Cmd

	res := resolveTemplatedCmd(cmd, cfg.Model)
	if len(cfg.ExtraArgs) > 0 {
		res = append(res, resolveTemplatedCmd(cfg.ExtraArgs, cfg.Model)...)
	}
	return res, nil
}

func resolveTemplatedCmd(cmd []string, model string) []string {
	if len(cmd) == 0 {
		return nil
	}
	res := make([]string, len(cmd))
	for i, arg := range cmd {
		res[i] = strings.ReplaceAll(arg, "{{.Model}}", model)
	}
	return res
}
