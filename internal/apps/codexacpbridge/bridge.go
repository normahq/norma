package codexacpbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"github.com/normahq/norma/internal/apps/appio"
	"github.com/normahq/norma/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
)

// DefaultAgentName is the fallback ACP agent name used when MCP identity is unavailable.
const DefaultAgentName = "norma-codex-acp-bridge"

// DefaultAgentVersion is the fallback ACP agent version used when MCP identity is unavailable.
const DefaultAgentVersion = "dev"

type codexMCPToolSession interface {
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	ListTools(ctx context.Context, params *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	InitializeResult() *mcp.InitializeResult
	Close() error
	Wait() error
}

type codexMCPToolSessionFactory func(ctx context.Context, cwd string) (codexMCPToolSession, error)

// RunProxy starts a Codex MCP server and exposes it as an ACP agent over stdio.
func RunProxy(ctx context.Context, workingDir string, opts Options, stdin io.Reader, stdout, stderr io.Writer) error {
	if stdin == nil {
		return errors.New("stdin is required")
	}
	if stdout == nil {
		return errors.New("stdout is required")
	}
	if stderr == nil {
		return errors.New("stderr is required")
	}
	if err := opts.validate(); err != nil {
		return err
	}
	lockedStderr := appio.NewSyncWriter(stderr)
	logger := logging.Ctx(ctx)

	command := buildCodexMCPCommand(opts)
	cmdName, cmdArgs := splitCommandForLog(command)
	requestedAgentName := strings.TrimSpace(opts.Name)
	bridgeClientName := requestedAgentName
	if bridgeClientName == "" {
		bridgeClientName = DefaultAgentName
	}
	logger.Debug().
		Str("working_dir", workingDir).
		Str("agent_name", bridgeClientName).
		Str("cmd", cmdName).
		Strs("args", cmdArgs).
		Msg("starting codex acp bridge")

	sessionFactory := func(factoryCtx context.Context, sessionCWD string) (codexMCPToolSession, error) {
		return connectCodexMCPProxySession(factoryCtx, workingDir, sessionCWD, command, bridgeClientName, lockedStderr, logger)
	}
	identity, err := validateCodexMCPFactory(ctx, sessionFactory, workingDir, logger)
	if err != nil {
		logger.Error().Err(err).Msg("required codex tools validation failed")
		return err
	}
	agentName, agentVersion := resolveAgentIdentity(requestedAgentName, identity)
	logger.Debug().
		Str("resolved_agent_name", agentName).
		Str("resolved_agent_version", agentVersion).
		Msg("resolved acp agent identity")

	proxy := newCodexACPProxyAgentWithFactory(sessionFactory, agentName, opts.codexToolConfig(), logger)
	proxy.setAgentVersion(agentVersion)
	conn := acp.NewAgentSideConnection(proxy, stdout, stdin)
	conn.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	proxy.setConnection(conn)
	logger.Debug().Msg("acp connection initialized")

	select {
	case <-conn.Done():
		logger.Debug().Msg("acp client disconnected")
		proxy.closeAllSessionBackends()
		return nil
	case <-ctx.Done():
		logger.Debug().Err(ctx.Err()).Msg("proxy context canceled")
		proxy.closeAllSessionBackends()
		return ctx.Err()
	}
}

func buildCodexMCPCommand(opts Options) []string {
	_ = opts
	command := make([]string, 0, 2)
	command = append(command, "codex", "mcp-server")
	return command
}

func validateCodexMCPFactory(ctx context.Context, factory codexMCPToolSessionFactory, cwd string, logger *zerolog.Logger) (mcpServerIdentity, error) {
	session, err := factory(ctx, cwd)
	if err != nil {
		return mcpServerIdentity{}, err
	}
	defer func() {
		_ = session.Close()
		_ = awaitBackendStop(session)
	}()
	if err := ensureCodexProxyTools(ctx, session, logger); err != nil {
		return mcpServerIdentity{}, err
	}
	return parseMCPServerIdentity(session.InitializeResult()), nil
}

func connectCodexMCPProxySession(
	ctx context.Context,
	workingDir string,
	sessionCWD string,
	command []string,
	agentName string,
	stderr io.Writer,
	logger *zerolog.Logger,
) (codexMCPToolSession, error) {
	if len(command) == 0 {
		return nil, errors.New("empty codex command")
	}
	cmdName, cmdArgs := splitCommandForLog(command)
	client := mcp.NewClient(&mcp.Implementation{Name: agentName, Version: "v0.0.1"}, nil)
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = strings.TrimSpace(sessionCWD)
	if cmd.Dir == "" {
		cmd.Dir = workingDir
	}
	cmd.Stderr = stderr
	logger.Debug().
		Str("cwd", cmd.Dir).
		Str("cmd", cmdName).
		Strs("args", cmdArgs).
		Msg("connecting mcp command transport")
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to mcp command: %w", err)
	}
	logger.Debug().Msg("connected to codex mcp session")
	return session, nil
}

func splitCommandForLog(command []string) (string, []string) {
	if len(command) == 0 {
		return "", nil
	}
	args := append([]string(nil), command[1:]...)
	return command[0], args
}

func ensureCodexProxyTools(ctx context.Context, session codexMCPToolSession, logger *zerolog.Logger) error {
	logger.Debug().
		Str("proto", "mcp").
		Str("method", "tools/list").
		Str("phase", "request").
		Msg("mcp event")
	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		logger.Error().
			Str("proto", "mcp").
			Str("method", "tools/list").
			Str("phase", "error").
			Err(err).
			Msg("mcp event")
		return fmt.Errorf("list mcp tools: %w", err)
	}
	if toolsResult == nil || len(toolsResult.Tools) == 0 {
		return errors.New("mcp tools list is empty")
	}
	toolNames := make([]string, 0, len(toolsResult.Tools))
	for _, t := range toolsResult.Tools {
		if t == nil {
			continue
		}
		toolNames = append(toolNames, t.Name)
	}
	logger.Debug().
		Str("proto", "mcp").
		Str("method", "tools/list").
		Str("phase", "response").
		Int("tool_count", len(toolNames)).
		Strs("tools", toolNames).
		Msg("mcp event")
	if logger.Debug().Enabled() {
		logger.Debug().
			Str("proto", "mcp").
			Str("method", "tools/list").
			Str("phase", "response_payload").
			Str("payload", logJSON(toolsResult)).
			Msg("mcp event")
	}
	seen := map[string]bool{}
	for _, t := range toolsResult.Tools {
		if t == nil {
			continue
		}
		seen[t.Name] = true
	}
	if !seen["codex"] || !seen["codex-reply"] {
		return fmt.Errorf("required tools not found (codex=%t codex-reply=%t)", seen["codex"], seen["codex-reply"])
	}
	logger.Debug().Msg("required codex tools are available")
	return nil
}
