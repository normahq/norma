package codexacpbridge

import (
	"context"
	"fmt"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

const (
	backendRestartReasonSessionNew      = "session_new"
	backendRestartReasonSessionSetModel = "session_set_model"
	backendRestartReasonSessionSetMode  = "session_set_mode"
	backendRestartReasonSessionRecreate = "session_recreate"
)

func (a *codexACPProxyAgent) setSessionConfig(
	sessionID acp.SessionId,
	reason string,
	apply func(*codexProxySessionState) bool,
) (codexMCPToolSession, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.sessions[sessionID]
	if !ok {
		return nil, false, acp.NewInvalidParams("session not found")
	}
	if state.cancel != nil {
		return nil, false, acp.NewInvalidRequest("cannot update session config while prompt is active")
	}
	backend := state.backend
	changed := apply(state)
	if changed {
		state.thread = ""
		state.backend = nil
		state.reason = normalizeBackendRestartReason(reason)
		a.logger.Debug().
			Str("session_id", string(sessionID)).
			Str("reason", state.reason).
			Str("cwd", strings.TrimSpace(state.cwd)).
			Str("model", strings.TrimSpace(state.model)).
			Int("mcp_server_count", len(state.mcpServers)).
			Bool("had_backend", backend != nil).
			Msg("mcp backend restart requested")
	}
	return backend, changed, nil
}

func (a *codexACPProxyAgent) ensureSessionBackend(ctx context.Context, sessionID acp.SessionId) error {
	a.mu.Lock()
	state, ok := a.sessions[sessionID]
	if !ok {
		a.mu.Unlock()
		return acp.NewInvalidParams("session not found")
	}
	if state.backend != nil {
		a.mu.Unlock()
		return nil
	}
	sessionCWD := state.cwd
	sessionModel := state.model
	sessionMCPCount := len(state.mcpServers)
	reason := normalizeBackendRestartReason(state.reason)
	a.mu.Unlock()
	a.logger.Debug().
		Str("session_id", string(sessionID)).
		Str("reason", reason).
		Str("cwd", strings.TrimSpace(sessionCWD)).
		Str("model", strings.TrimSpace(sessionModel)).
		Int("mcp_server_count", sessionMCPCount).
		Msg("starting mcp backend session")

	backend, err := a.sessionFactory(ctx, sessionCWD)
	if err != nil {
		return fmt.Errorf("create codex session backend: %w", err)
	}
	if err := ensureCodexProxyTools(ctx, backend, a.logger); err != nil {
		_ = backend.Close()
		_ = awaitBackendStop(backend)
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	state, ok = a.sessions[sessionID]
	if !ok {
		_ = backend.Close()
		_ = awaitBackendStop(backend)
		return acp.NewInvalidParams("session not found")
	}
	if state.backend != nil {
		_ = backend.Close()
		_ = awaitBackendStop(backend)
		a.logger.Debug().
			Str("session_id", string(sessionID)).
			Str("reason", reason).
			Msg("discarded recreated mcp backend because session already has backend")
		return nil
	}
	state.backend = backend
	state.reason = backendRestartReasonSessionRecreate
	a.logger.Debug().
		Str("session_id", string(sessionID)).
		Str("reason", reason).
		Str("cwd", strings.TrimSpace(state.cwd)).
		Str("model", strings.TrimSpace(state.model)).
		Int("mcp_server_count", len(state.mcpServers)).
		Msg("mcp backend session ready")
	return nil
}

func (a *codexACPProxyAgent) closeBackendForRestart(sessionID acp.SessionId, backend codexMCPToolSession, reason string) {
	if backend == nil {
		return
	}
	normalizedReason := normalizeBackendRestartReason(reason)
	a.logger.Debug().
		Str("session_id", string(sessionID)).
		Str("reason", normalizedReason).
		Msg("closing mcp backend for restart")
	if err := backend.Close(); err != nil {
		event := a.logger.Warn()
		if isExpectedBackendShutdownErr(err) {
			event = a.logger.Debug()
		}
		event.
			Err(err).
			Str("session_id", string(sessionID)).
			Str("reason", normalizedReason).
			Msg("failed to close session backend")
	}
	if err := awaitBackendStop(backend); err != nil {
		event := a.logger.Warn()
		if isExpectedBackendShutdownErr(err) {
			event = a.logger.Debug()
		}
		event.
			Err(err).
			Str("session_id", string(sessionID)).
			Str("reason", normalizedReason).
			Msg("failed waiting for session backend stop")
		return
	}
	a.logger.Debug().
		Str("session_id", string(sessionID)).
		Str("reason", normalizedReason).
		Msg("closed mcp backend for restart")
}

func (a *codexACPProxyAgent) closeAllSessionBackends() {
	type backendEntry struct {
		sessionID acp.SessionId
		backend   codexMCPToolSession
	}
	entries := make([]backendEntry, 0)

	a.mu.Lock()
	for sessionID, state := range a.sessions {
		if state.cancel != nil {
			state.cancel()
			state.cancel = nil
		}
		if state.backend != nil {
			entries = append(entries, backendEntry{sessionID: sessionID, backend: state.backend})
			state.backend = nil
		}
	}
	a.mu.Unlock()

	for _, entry := range entries {
		if err := entry.backend.Close(); err != nil {
			event := a.logger.Warn()
			if isExpectedBackendShutdownErr(err) {
				event = a.logger.Debug()
			}
			event.Err(err).Str("session_id", string(entry.sessionID)).Msg("failed to close session backend")
		}
		if err := awaitBackendStop(entry.backend); err != nil {
			event := a.logger.Warn()
			if isExpectedBackendShutdownErr(err) {
				event = a.logger.Debug()
			}
			event.Err(err).Str("session_id", string(entry.sessionID)).Msg("failed waiting for session backend stop")
		}
	}
}

func isExpectedBackendShutdownErr(err error) bool {
	if err == nil {
		return false
	}
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "signal: killed") ||
		strings.Contains(lowered, "process already finished")
}

func awaitBackendStop(backend codexMCPToolSession) error {
	return backend.Wait()
}

func normalizeBackendRestartReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return backendRestartReasonSessionRecreate
	}
	return trimmed
}
