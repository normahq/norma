package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	methodInitialize        = "initialize"
	methodSessionNew        = "session/new"
	methodSessionPrompt     = "session/prompt"
	methodSessionCancel     = "session/cancel"
	methodSessionUpdate     = "session/update"
	updateAgentMessageChunk = "agent_message_chunk"
)

func TestPlaygroundCommandRegistered(t *testing.T) {
	cmd := playgroundCmd()
	sub, _, err := cmd.Find([]string{"gemini-acp"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if sub == nil || sub.Name() != "gemini-acp" {
		t.Fatalf("subcommand = %v, want gemini-acp", sub)
	}
}

func TestRunGeminiACPOneShot(t *testing.T) {
	wrapper, argsFile := writeGeminiWrapper(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runGeminiACP(context.Background(), t.TempDir(), geminiACPOptions{
		Prompt:     "hello",
		Model:      "gemini-test",
		GeminiBin:  wrapper,
		GeminiArgs: []string{"--sandbox", "workspace-write"},
	}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGeminiACP() error = %v", err)
	}

	if got := stdout.String(); got != "session-1:hello\n" {
		t.Fatalf("stdout = %q, want %q", got, "session-1:hello\n")
	}
	args := readArgsFile(t, argsFile)
	wantArgs := []string{"--experimental-acp", "--model", "gemini-test", "--sandbox", "workspace-write"}
	for _, want := range wantArgs {
		if !containsArg(args, want) {
			t.Fatalf("args %v do not contain %q", args, want)
		}
	}
}

func TestRunGeminiACPReusesSessionInREPL(t *testing.T) {
	wrapper, _ := writeGeminiWrapper(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	input := strings.NewReader("first\nsecond\nquit\n")
	err := runGeminiACP(context.Background(), t.TempDir(), geminiACPOptions{GeminiBin: wrapper}, input, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGeminiACP() error = %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "session-1:first") {
		t.Fatalf("stdout = %q, want first response", got)
	}
	if !strings.Contains(got, "session-1:second") {
		t.Fatalf("stdout = %q, want second response", got)
	}
	if strings.Contains(got, "session-2") {
		t.Fatalf("stdout = %q, want single ACP session reuse", got)
	}
}

func writeGeminiWrapper(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	wrapperPath := filepath.Join(dir, "gemini-wrapper.sh")
	script := fmt.Sprintf(`#!/bin/sh
: > %s
for arg in "$@"; do
  printf '%%s\n' "$arg" >> %s
done
exec env GO_WANT_PLAYGROUND_ACP_HELPER=1 %s -test.run=TestPlaygroundACPHelperProcess -- "$@"
`, shellQuote(argsFile), shellQuote(argsFile), shellQuote(os.Args[0]))
	if err := os.WriteFile(wrapperPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", wrapperPath, err)
	}
	return wrapperPath, argsFile
}

func readArgsFile(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func TestPlaygroundACPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_PLAYGROUND_ACP_HELPER") != "1" {
		return
	}
	runPlaygroundACPHelper(os.Stdin, os.Stdout)
	os.Exit(0)
}

func runPlaygroundACPHelper(stdin *os.File, stdout *os.File) {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	sessionCount := 0

	for scanner.Scan() {
		var msg helperEnvelope
		mustHelper(json.Unmarshal(scanner.Bytes(), &msg))
		switch msg.Method {
		case methodInitialize:
			writeHelperEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustHelperJSON(helperInitializeResponse{ProtocolVersion: 1})})
		case methodSessionNew:
			sessionCount++
			writeHelperEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustHelperJSON(helperNewSessionResponse{SessionID: fmt.Sprintf("session-%d", sessionCount)})})
		case methodSessionPrompt:
			var req helperPromptRequest
			mustHelper(json.Unmarshal(msg.Params, &req))
			writeHelperUpdate(stdout, req.SessionID, req.SessionID+":")
			writeHelperUpdate(stdout, req.SessionID, req.Prompt[0].Text)
			writeHelperEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", ID: msg.ID, Result: mustHelperJSON(helperPromptResponse{StopReason: "end_turn"})})
		case methodSessionCancel:
		default:
			writeHelperEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", ID: msg.ID, Error: &helperError{Code: -32601, Message: "unsupported"}})
		}
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

func writeHelperUpdate(stdout *os.File, sessionID, text string) {
	writeHelperEnvelope(stdout, helperEnvelope{JSONRPC: "2.0", Method: methodSessionUpdate, Params: mustHelperJSON(helperSessionNotification{
		SessionID: sessionID,
		Update:    helperSessionUpdate{SessionUpdate: updateAgentMessageChunk, Content: &helperContentPart{Type: "text", Text: text}},
	})})
}

func writeHelperEnvelope(stdout *os.File, env helperEnvelope) {
	mustHelper(json.NewEncoder(stdout).Encode(env))
}

func mustHelperJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	mustHelper(err)
	return data
}

func mustHelper(err error) {
	if err != nil {
		panic(err)
	}
}
