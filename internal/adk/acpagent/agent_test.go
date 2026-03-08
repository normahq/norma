package acpagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"strings"
	"testing"

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

	updates, resultCh, err := client.Prompt(context.Background(), sess.SessionID, "hello")
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
	if result.Response.StopReason != "end_turn" {
		t.Fatalf("StopReason = %q, want end_turn", result.Response.StopReason)
	}
	got := strings.Join(chunks, "")
	want := sess.SessionID + ":hello"
	if got != want {
		t.Fatalf("joined chunks = %q, want %q", got, want)
	}
}

func TestClientHandlesPermissionRequest(t *testing.T) {
	client, err := NewClient(context.Background(), ClientConfig{
		Command: helperCommand(t),
		PermissionHandler: func(_ context.Context, req requestPermissionRequest) (requestPermissionResponse, error) {
			return requestPermissionResponse{Outcome: permissionOutcome{Outcome: outcomeSelected, OptionID: req.Options[0].OptionID}}, nil
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

	updates, resultCh, err := client.Prompt(context.Background(), sess.SessionID, "permission")
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
	t.Helper()
	return []string{"env", "GO_WANT_ACP_HELPER=1", os.Args[0], "-test.run=TestACPHelperProcess", "--"}
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

	for scanner.Scan() {
		var msg rpcEnvelope
		must(json.Unmarshal(scanner.Bytes(), &msg))
		switch msg.Method {
		case methodInitialize:
			writeEnvelope(stdout, rpcEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustJSON(initializeResponse{ProtocolVersion: protocolVersion, AgentInfo: &implementation{Name: "helper"}})})
		case methodSessionNew:
			sessionCount++
			sessionID := fmt.Sprintf("session-%d", sessionCount)
			writeEnvelope(stdout, rpcEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustJSON(newSessionResponse{SessionID: sessionID})})
		case methodSessionPrompt:
			var req promptRequest
			must(json.Unmarshal(msg.Params, &req))
			prompt := req.Prompt[0].Text
			if prompt == "permission" {
				writeEnvelope(stdout, rpcEnvelope{JSONRPC: "2.0", ID: json.RawMessage(`"perm-1"`), Method: methodSessionRequestPermit, Params: mustJSON(requestPermissionRequest{
					SessionID: req.SessionID,
					ToolCall:  permissionToolCall{Title: "Edit file"},
					Options:   []permissionOption{{Kind: "allow_once", Name: "Allow", OptionID: "allow"}, {Kind: "reject_once", Name: "Reject", OptionID: "reject"}},
				})})
				if !scanner.Scan() {
					return
				}
				var permitResp rpcEnvelope
				must(json.Unmarshal(scanner.Bytes(), &permitResp))
				var outcome requestPermissionResponse
				must(json.Unmarshal(permitResp.Result, &outcome))
				text := "rejected"
				if outcome.Outcome.OptionID == "allow" {
					text = "approved"
				}
				writeUpdate(stdout, req.SessionID, text)
				writeEnvelope(stdout, rpcEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustJSON(promptResponse{StopReason: "end_turn"})})
				continue
			}
			prefix := req.SessionID + ":"
			writeUpdate(stdout, req.SessionID, prefix)
			writeUpdate(stdout, req.SessionID, prompt)
			writeEnvelope(stdout, rpcEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustJSON(promptResponse{StopReason: "end_turn"})})
		case methodSessionCancel:
			// Ignore in helper.
		default:
			writeEnvelope(stdout, rpcEnvelope{JSONRPC: "2.0", ID: msg.ID, Error: &rpcError{Code: -32601, Message: "unsupported"}})
		}
	}
}

func writeUpdate(stdout *os.File, sessionID, text string) {
	writeEnvelope(stdout, rpcEnvelope{JSONRPC: "2.0", Method: methodSessionUpdate, Params: mustJSON(sessionNotification{
		SessionID: sessionID,
		Update:    sessionUpdate{SessionUpdate: updateAgentMessageChunk, Content: &textContent{Type: "text", Text: text}},
	})})
}

func writeEnvelope(stdout *os.File, msg rpcEnvelope) {
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
