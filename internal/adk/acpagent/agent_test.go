package acpagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/rs/zerolog"
	"google.golang.org/adk/agent"
	runnerpkg "google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestClientPromptReceivesUpdates(t *testing.T) {
	client, err := NewClient(context.Background(), ClientConfig{
		Command: helperCommand(t),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	sess, err := client.NewSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	updates, resultCh, err := client.Prompt(context.Background(), string(sess.SessionId), "hello")
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	var chunks []string
	for note := range updates {
		if text := updateText(note.Update); text != "" {
			chunks = append(chunks, text)
		}
	}
	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("PromptResult.Err = %v", result.Err)
	}
	if result.Response.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", result.Response.StopReason, acp.StopReasonEndTurn)
	}
	got := strings.Join(chunks, "")
	want := string(sess.SessionId) + ":hello"
	if got != want {
		t.Fatalf("joined chunks = %q, want %q", got, want)
	}
}

func TestClientHandlesPermissionRequest(t *testing.T) {
	client, err := NewClient(context.Background(), ClientConfig{
		Command: helperCommand(t),
		PermissionHandler: func(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
			return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected(req.Options[0].OptionId)}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	sess, err := client.NewSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	updates, resultCh, err := client.Prompt(context.Background(), string(sess.SessionId), "permission")
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	var chunks []string
	for note := range updates {
		if text := updateText(note.Update); text != "" {
			chunks = append(chunks, text)
		}
	}
	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("PromptResult.Err = %v", result.Err)
	}
	if got := strings.Join(chunks, ""); got != "approved" {
		t.Fatalf("joined chunks = %q, want approved", got)
	}
}

func TestClientInitializeUsesDefaultIdentity(t *testing.T) {
	client, err := NewClient(context.Background(), ClientConfig{
		Command: helperCommandWithEnv(t, map[string]string{
			"GO_EXPECT_CLIENT_NAME":    "norma-acpagent",
			"GO_EXPECT_CLIENT_VERSION": "dev",
		}),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
}

func TestClientInitializeUsesConfiguredIdentity(t *testing.T) {
	client, err := NewClient(context.Background(), ClientConfig{
		Command: helperCommandWithEnv(t, map[string]string{
			"GO_EXPECT_CLIENT_NAME":    "custom-acp-client",
			"GO_EXPECT_CLIENT_VERSION": "1.2.3",
		}),
		ClientName:    "custom-acp-client",
		ClientVersion: "1.2.3",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
}

func TestClientPromptAllowsConcurrentDifferentSessions(t *testing.T) {
	const (
		wantSession1 = "session-1:one"
		wantSession2 = "session-2:two"
	)

	client, err := NewClient(context.Background(), ClientConfig{
		Command: helperCommand(t),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	sess1, err := client.NewSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	sess2, err := client.NewSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	updates1, resultCh1, err := client.Prompt(context.Background(), string(sess1.SessionId), "slow:one")
	if err != nil {
		t.Fatalf("Prompt(session1) error = %v", err)
	}
	updates2, resultCh2, err := client.Prompt(context.Background(), string(sess2.SessionId), "two")
	if err != nil {
		t.Fatalf("Prompt(session2) error = %v", err)
	}

	got1 := readPromptOutput(t, updates1, resultCh1)
	got2 := readPromptOutput(t, updates2, resultCh2)
	if got1 != wantSession1 {
		t.Fatalf("session1 output = %q, want %q", got1, wantSession1)
	}
	if got2 != wantSession2 {
		t.Fatalf("session2 output = %q, want %q", got2, wantSession2)
	}
}

func TestClientPromptRejectsConcurrentSameSession(t *testing.T) {
	const wantSession1 = "session-1:one"

	client, err := NewClient(context.Background(), ClientConfig{
		Command: helperCommand(t),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	sess, err := client.NewSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	updates1, resultCh1, err := client.Prompt(context.Background(), string(sess.SessionId), "slow:one")
	if err != nil {
		t.Fatalf("Prompt(first) error = %v", err)
	}
	if _, _, err := client.Prompt(context.Background(), string(sess.SessionId), "two"); !errors.Is(err, errPromptAlreadyActive) {
		t.Fatalf("Prompt(second) error = %v, want %v", err, errPromptAlreadyActive)
	}

	got1 := readPromptOutput(t, updates1, resultCh1)
	if got1 != wantSession1 {
		t.Fatalf("session output = %q, want %q", got1, wantSession1)
	}
}

func TestRequestPermissionPassesContextToHandler(t *testing.T) {
	type key string
	const ctxKey key = "ctx-key"
	const ctxVal = "ctx-value"

	var seen string
	c := &Client{
		logger: zerolog.Nop(),
		permissionHandler: func(ctx context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
			seen, _ = ctx.Value(ctxKey).(string)
			return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
		},
	}

	_, err := c.RequestPermission(context.WithValue(context.Background(), ctxKey, ctxVal), acp.RequestPermissionRequest{
		SessionId: "session-1",
		Options:   []acp.PermissionOption{},
	})
	if err != nil {
		t.Fatalf("RequestPermission() error = %v", err)
	}
	if seen != ctxVal {
		t.Fatalf("permission handler context value = %q, want %q", seen, ctxVal)
	}
}

func TestAgentReusesRemoteSession(t *testing.T) {
	a, err := New(Config{
		Context:    context.Background(),
		Command:    helperCommand(t),
		WorkingDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = a.Close() }()

	sessionService := session.InMemoryService()
	r, err := runnerpkg.New(runnerpkg.Config{
		AppName:        "test-app",
		Agent:          a,
		SessionService: sessionService,
	})
	if err != nil {
		t.Fatalf("runner.New() error = %v", err)
	}
	sess, err := sessionService.Create(context.Background(), &session.CreateRequest{AppName: "test-app", UserID: "test-user"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	first := collectFinalText(t, r.Run(context.Background(), "test-user", sess.Session.ID(), genai.NewContentFromText("one", genai.RoleUser), agent.RunConfig{}))
	second := collectFinalText(t, r.Run(context.Background(), "test-user", sess.Session.ID(), genai.NewContentFromText("two", genai.RoleUser), agent.RunConfig{}))

	if first != "session-1:one" {
		t.Fatalf("first final text = %q, want session-1:one", first)
	}
	if second != "session-1:two" {
		t.Fatalf("second final text = %q, want session-1:two", second)
	}
}

func collectFinalText(t *testing.T, events iter.Seq2[*session.Event, error]) string {
	t.Helper()
	final := ""
	for ev, err := range events {
		if err != nil {
			t.Fatalf("runner event error = %v", err)
		}
		if ev == nil || ev.Content == nil || ev.Partial {
			continue
		}
		final = extractPromptText(ev.Content)
	}
	return final
}

func helperCommand(t *testing.T) []string {
	return helperCommandWithEnv(t, nil)
}

func helperCommandWithEnv(t *testing.T, env map[string]string) []string {
	t.Helper()
	cmd := []string{"env", "GO_WANT_ACP_HELPER=1"}
	if len(env) > 0 {
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			cmd = append(cmd, key+"="+env[key])
		}
	}
	cmd = append(cmd, os.Args[0], "-test.run=TestACPHelperProcess", "--")
	return cmd
}

func TestACPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_ACP_HELPER") != "1" {
		return
	}
	runACPHelper(os.Stdin, os.Stdout)
	os.Exit(0)
}

func runACPHelper(stdin *os.File, stdout *os.File) {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	sessionCount := 0
	expectedClientName := os.Getenv("GO_EXPECT_CLIENT_NAME")
	expectedClientVersion := os.Getenv("GO_EXPECT_CLIENT_VERSION")

	for scanner.Scan() {
		var msg helperEnvelope
		must(json.Unmarshal(scanner.Bytes(), &msg))
		switch msg.Method {
		case acp.AgentMethodInitialize:
			var req helperInitializeRequest
			must(json.Unmarshal(msg.Params, &req))
			if expectedClientName != "" && req.ClientInfo.Name != expectedClientName {
				writeEnvelope(stdout, helperEnvelope{
					JSONRPC: "2.0",
					ID:      msg.ID,
					Error:   &helperError{Code: -32000, Message: fmt.Sprintf("unexpected client name: %s", req.ClientInfo.Name)},
				})
				continue
			}
			if expectedClientVersion != "" && req.ClientInfo.Version != expectedClientVersion {
				writeEnvelope(stdout, helperEnvelope{
					JSONRPC: "2.0",
					ID:      msg.ID,
					Error:   &helperError{Code: -32000, Message: fmt.Sprintf("unexpected client version: %s", req.ClientInfo.Version)},
				})
				continue
			}
			writeEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustJSON(helperInitializeResponse{ProtocolVersion: acp.ProtocolVersionNumber})})
		case acp.AgentMethodSessionNew:
			sessionCount++
			sessionID := fmt.Sprintf("session-%d", sessionCount)
			writeEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustJSON(helperNewSessionResponse{SessionID: sessionID})})
		case acp.AgentMethodSessionPrompt:
			var req helperPromptRequest
			must(json.Unmarshal(msg.Params, &req))
			prompt := req.Prompt[0].Text
			if strings.HasPrefix(prompt, "slow:") {
				time.Sleep(150 * time.Millisecond)
				prompt = strings.TrimPrefix(prompt, "slow:")
			}
			if prompt == "permission" {
				title := "Edit file"
				writeEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", ID: json.RawMessage(`"perm-1"`), Method: acp.ClientMethodSessionRequestPermission, Params: mustJSON(helperPermissionRequest{
					SessionID: req.SessionID,
					ToolCall:  helperPermissionToolCall{Title: &title},
					Options: []helperPermissionOption{
						{Kind: string(acp.PermissionOptionKindAllowOnce), Name: "Allow", OptionID: "allow"},
						{Kind: string(acp.PermissionOptionKindRejectOnce), Name: "Reject", OptionID: "reject"},
					},
				})})
				if !scanner.Scan() {
					return
				}
				var permitResp helperEnvelope
				must(json.Unmarshal(scanner.Bytes(), &permitResp))
				var outcome helperPermissionResponse
				must(json.Unmarshal(permitResp.Result, &outcome))
				text := "rejected"
				if outcome.Outcome.Outcome == "selected" && outcome.Outcome.OptionID == "allow" {
					text = "approved"
				}
				writeUpdate(stdout, req.SessionID, text)
				writeEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustJSON(helperPromptResponse{StopReason: string(acp.StopReasonEndTurn)})})
				continue
			}
			prefix := req.SessionID + ":"
			writeUpdate(stdout, req.SessionID, prefix)
			writeUpdate(stdout, req.SessionID, prompt)
			writeEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustJSON(helperPromptResponse{StopReason: string(acp.StopReasonEndTurn)})})
		case acp.AgentMethodSessionCancel:
			// Ignore in helper.
		default:
			writeEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", ID: msg.ID, Error: &helperError{Code: -32601, Message: "unsupported"}})
		}
	}
}

func writeUpdate(stdout *os.File, sessionID, text string) {
	writeEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", Method: acp.ClientMethodSessionUpdate, Params: mustJSON(helperSessionNotification{
		SessionID: sessionID,
		Update: helperSessionUpdate{
			SessionUpdate: "agent_message_chunk",
			Content:       &helperContentPart{Type: "text", Text: text},
		},
	})})
}

func writeEnvelope(stdout *os.File, msg helperEnvelope) {
	must(json.NewEncoder(stdout).Encode(msg))
}

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	must(err)
	return data
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

type helperEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *helperError    `json:"error,omitempty"`
}

type helperError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type helperInitializeResponse struct {
	ProtocolVersion int `json:"protocolVersion"`
}

type helperInitializeRequest struct {
	ClientInfo helperImplementation `json:"clientInfo"`
}

type helperImplementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type helperNewSessionResponse struct {
	SessionID string `json:"sessionId"`
}

type helperPromptResponse struct {
	StopReason string `json:"stopReason"`
}

type helperPromptRequest struct {
	SessionID string              `json:"sessionId"`
	Prompt    []helperContentPart `json:"prompt"`
}

type helperContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type helperSessionNotification struct {
	SessionID string              `json:"sessionId"`
	Update    helperSessionUpdate `json:"update"`
}

type helperSessionUpdate struct {
	SessionUpdate string             `json:"sessionUpdate"`
	Content       *helperContentPart `json:"content,omitempty"`
}

type helperPermissionRequest struct {
	SessionID string                   `json:"sessionId"`
	Options   []helperPermissionOption `json:"options"`
	ToolCall  helperPermissionToolCall `json:"toolCall"`
}

type helperPermissionOption struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	OptionID string `json:"optionId"`
}

type helperPermissionToolCall struct {
	Title *string `json:"title,omitempty"`
}

type helperPermissionResponse struct {
	Outcome helperPermissionOutcome `json:"outcome"`
}

type helperPermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

func readPromptOutput(t *testing.T, updates <-chan acp.SessionNotification, resultCh <-chan PromptResult) string {
	t.Helper()
	var chunks []string
	for note := range updates {
		if text := updateText(note.Update); text != "" {
			chunks = append(chunks, text)
		}
	}
	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("PromptResult.Err = %v", result.Err)
	}
	return strings.Join(chunks, "")
}
