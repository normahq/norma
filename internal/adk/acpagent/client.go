package acpagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

var errPromptAlreadyActive = errors.New("acp prompt already active")

type PermissionHandler func(context.Context, requestPermissionRequest) (requestPermissionResponse, error)

type TracefFunc func(format string, args ...any)

type ClientConfig struct {
	Command           []string
	WorkingDir        string
	Stderr            io.Writer
	PermissionHandler PermissionHandler
	Tracef            TracefFunc
}

type Client struct {
	cmd               *exec.Cmd
	stdin             io.WriteCloser
	permissionHandler PermissionHandler
	traceFn           TracefFunc

	writeMu   sync.Mutex
	stateMu   sync.Mutex
	pending   map[string]chan rpcEnvelope
	active    *activePrompt
	nextID    atomic.Uint64
	closed    chan struct{}
	closeOnce sync.Once
	closeErr  error
}

type activePrompt struct {
	sessionID string
	updates   chan sessionNotification
}

type PromptResult struct {
	Response promptResponse
	Err      error
}

func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("acp command is required")
	}

	stderr := cfg.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	cmd := exec.CommandContext(ctx, cfg.Command[0], cfg.Command[1:]...)
	cmd.Dir = cfg.WorkingDir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acp stdout pipe: %w", err)
	}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start acp process: %w", err)
	}

	c := &Client{
		cmd:               cmd,
		stdin:             stdin,
		permissionHandler: cfg.PermissionHandler,
		traceFn:           cfg.Tracef,
		pending:           make(map[string]chan rpcEnvelope),
		closed:            make(chan struct{}),
	}
	go c.readLoop(stdout)
	go c.waitLoop()
	return c, nil
}

func (c *Client) Initialize(ctx context.Context) (initializeResponse, error) {
	resp, err := c.request(ctx, methodInitialize, initializeRequest{
		ProtocolVersion: protocolVersion,
		ClientInfo: &implementation{
			Name:    "norma-playground",
			Version: "dev",
		},
	})
	if err != nil {
		return initializeResponse{}, err
	}

	var out initializeResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		return initializeResponse{}, fmt.Errorf("decode initialize response: %w", err)
	}
	if out.ProtocolVersion != protocolVersion {
		return initializeResponse{}, fmt.Errorf("unsupported acp protocol version %d", out.ProtocolVersion)
	}
	return out, nil
}

func (c *Client) Authenticate(ctx context.Context, methodID string) error {
	if strings.TrimSpace(methodID) == "" {
		return nil
	}
	_, err := c.request(ctx, methodAuthenticate, authenticateRequest{MethodID: methodID})
	return err
}

func (c *Client) NewSession(ctx context.Context, cwd string) (newSessionResponse, error) {
	resp, err := c.request(ctx, methodSessionNew, newSessionRequest{Cwd: cwd, MCPServers: []mcpServer{}})
	if err != nil {
		return newSessionResponse{}, err
	}

	var out newSessionResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		return newSessionResponse{}, fmt.Errorf("decode new session response: %w", err)
	}
	if strings.TrimSpace(out.SessionID) == "" {
		return newSessionResponse{}, fmt.Errorf("acp session id is empty")
	}
	return out, nil
}

func (c *Client) Prompt(ctx context.Context, sessionID, prompt string) (<-chan sessionNotification, <-chan PromptResult, error) {
	c.stateMu.Lock()
	if c.active != nil {
		c.stateMu.Unlock()
		return nil, nil, errPromptAlreadyActive
	}
	updates := make(chan sessionNotification, 64)
	c.active = &activePrompt{sessionID: sessionID, updates: updates}
	c.stateMu.Unlock()

	resultCh := make(chan PromptResult, 1)
	go func() {
		defer close(resultCh)
		defer close(updates)
		defer c.clearActive(sessionID)

		cancelCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			select {
			case <-ctx.Done():
				_ = c.notify(cancelCtx, methodSessionCancel, cancelNotification{SessionID: sessionID})
			case <-cancelCtx.Done():
			}
		}()

		resp, err := c.request(ctx, methodSessionPrompt, promptRequest{
			SessionID: sessionID,
			Prompt: []contentBlock{{
				Type: "text",
				Text: prompt,
			}},
		})
		if err != nil {
			resultCh <- PromptResult{Err: err}
			return
		}

		var out promptResponse
		if err := json.Unmarshal(resp, &out); err != nil {
			resultCh <- PromptResult{Err: fmt.Errorf("decode prompt response: %w", err)}
			return
		}
		resultCh <- PromptResult{Response: out}
	}()

	return updates, resultCh, nil
}

func (c *Client) Close() error {
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	<-c.closed
	if c.closeErr != nil && !errors.Is(c.closeErr, io.EOF) {
		return c.closeErr
	}
	return nil
}

func (c *Client) waitLoop() {
	err := c.cmd.Wait()
	if err != nil {
		c.failAll(fmt.Errorf("acp process exit: %w", err))
		return
	}
	c.failAll(io.EOF)
}

func (c *Client) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg rpcEnvelope
		if err := json.Unmarshal(line, &msg); err != nil {
			c.failAll(fmt.Errorf("decode acp message: %w", err))
			return
		}
		c.tracef("recv %s %s", msg.Method, strings.TrimSpace(string(msg.ID)))
		if msg.Method != "" {
			if len(msg.ID) > 0 {
				c.handleRequest(msg)
				continue
			}
			c.handleNotification(msg)
			continue
		}
		c.handleResponse(msg)
	}
	if err := scanner.Err(); err != nil {
		c.failAll(fmt.Errorf("read acp stdout: %w", err))
	}
}

func (c *Client) handleNotification(msg rpcEnvelope) {
	if msg.Method != methodSessionUpdate {
		c.tracef("ignore notification %s", msg.Method)
		return
	}
	var note sessionNotification
	if err := json.Unmarshal(msg.Params, &note); err != nil {
		c.tracef("invalid session update: %v", err)
		return
	}
	c.tracef("session update %s %s", note.SessionID, note.Update.SessionUpdate)

	c.stateMu.Lock()
	active := c.active
	c.stateMu.Unlock()
	if active == nil || active.sessionID != note.SessionID {
		return
	}
	select {
	case active.updates <- note:
	case <-c.closed:
	}
}

func (c *Client) handleRequest(msg rpcEnvelope) {
	switch msg.Method {
	case methodSessionRequestPermit:
		var req requestPermissionRequest
		if err := json.Unmarshal(msg.Params, &req); err != nil {
			c.respondError(msg.ID, -32602, fmt.Sprintf("invalid permission request: %v", err))
			return
		}
		resp, err := c.handlePermissionRequest(req)
		if err != nil {
			c.respondError(msg.ID, -32000, err.Error())
			return
		}
		c.respondResult(msg.ID, resp)
	default:
		c.respondError(msg.ID, -32601, fmt.Sprintf("unsupported client method %q", msg.Method))
	}
}

func (c *Client) handlePermissionRequest(req requestPermissionRequest) (requestPermissionResponse, error) {
	c.tracef("permission request %s %s", req.SessionID, req.ToolCall.Title)
	if c.permissionHandler != nil {
		return c.permissionHandler(context.Background(), req)
	}
	for _, option := range req.Options {
		if option.Kind == "reject_once" || option.Kind == "reject_always" {
			return requestPermissionResponse{Outcome: permissionOutcome{Outcome: outcomeSelected, OptionID: option.OptionID}}, nil
		}
	}
	return requestPermissionResponse{Outcome: permissionOutcome{Outcome: outcomeCancelled}}, nil
}

func (c *Client) handleResponse(msg rpcEnvelope) {
	key := string(msg.ID)
	c.stateMu.Lock()
	ch := c.pending[key]
	delete(c.pending, key)
	c.stateMu.Unlock()
	if ch == nil {
		return
	}
	ch <- msg
	close(ch)
}

func (c *Client) clearActive(sessionID string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.active != nil && c.active.sessionID == sessionID {
		c.active = nil
	}
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := fmt.Sprintf("%d", c.nextID.Add(1))
	idRaw, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("marshal request id: %w", err)
	}

	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal %s params: %w", method, err)
	}

	respCh := make(chan rpcEnvelope, 1)
	c.stateMu.Lock()
	c.pending[string(idRaw)] = respCh
	c.stateMu.Unlock()
	defer func() {
		c.stateMu.Lock()
		delete(c.pending, string(idRaw))
		c.stateMu.Unlock()
	}()

	if err := c.send(rpcEnvelope{
		JSONRPC: "2.0",
		ID:      idRaw,
		Method:  method,
		Params:  paramsRaw,
	}); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		if c.closeErr == nil {
			return nil, io.EOF
		}
		return nil, c.closeErr
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal %s params: %w", method, err)
	}
	return c.send(rpcEnvelope{JSONRPC: "2.0", Method: method, Params: paramsRaw})
}

func (c *Client) send(msg rpcEnvelope) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal acp message: %w", err)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.closed:
		if c.closeErr == nil {
			return io.EOF
		}
		return c.closeErr
	default:
	}
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write acp message: %w", err)
	}
	c.tracef("send %s %s", msg.Method, strings.TrimSpace(string(msg.ID)))
	return nil
}

func (c *Client) respondResult(id json.RawMessage, result any) {
	resultRaw, err := json.Marshal(result)
	if err != nil {
		c.respondError(id, -32603, err.Error())
		return
	}
	_ = c.send(rpcEnvelope{JSONRPC: "2.0", ID: id, Result: resultRaw})
}

func (c *Client) respondError(id json.RawMessage, code int, message string) {
	_ = c.send(rpcEnvelope{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func (c *Client) failAll(err error) {
	c.closeOnce.Do(func() {
		c.closeErr = err
		close(c.closed)
		c.stateMu.Lock()
		pending := c.pending
		c.pending = make(map[string]chan rpcEnvelope)
		c.active = nil
		c.stateMu.Unlock()
		for _, ch := range pending {
			ch <- rpcEnvelope{Error: &rpcError{Code: -32000, Message: err.Error()}}
			close(ch)
		}
	})
}

func (c *Client) tracef(format string, args ...any) {
	if c.traceFn != nil {
		c.traceFn(format, args...)
	}
}
