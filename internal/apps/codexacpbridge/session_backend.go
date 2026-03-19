package codexacpbridge

import (
	"context"
	"fmt"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

func (a *codexACPProxyAgent) setSessionConfig(
	sessionID acp.SessionId,
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
	a.mu.Unlock()

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
		return nil
	}
	state.backend = backend
	return nil
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
