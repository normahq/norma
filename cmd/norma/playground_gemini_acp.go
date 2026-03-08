package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/metalagman/norma/internal/adk/acpagent"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"google.golang.org/adk/agent"
	runnerpkg "google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

type geminiACPOptions struct {
	Prompt      string
	Model       string
	GeminiBin   string
	GeminiArgs  []string
	DebugEvents bool
}

func playgroundGeminiACPCmd() *cobra.Command {
	opts := geminiACPOptions{GeminiBin: "gemini"}
	cmd := &cobra.Command{
		Use:          "gemini-acp",
		Short:        "Run Gemini CLI in ACP mode through Go ADK",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			return runGeminiACP(cmd.Context(), repoRoot, opts, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&opts.Prompt, "prompt", "", "single prompt to run; if empty starts a REPL")
	cmd.Flags().StringVar(&opts.Model, "model", "", "Gemini model name")
	cmd.Flags().StringVar(&opts.GeminiBin, "gemini-bin", opts.GeminiBin, "Gemini executable path")
	cmd.Flags().StringArrayVar(&opts.GeminiArgs, "gemini-arg", nil, "extra Gemini CLI argument (repeatable)")
	cmd.Flags().BoolVar(&opts.DebugEvents, "debug-events", false, "print ACP event summaries to stderr")
	return cmd
}

func runGeminiACP(ctx context.Context, repoRoot string, opts geminiACPOptions, stdin io.Reader, stdout, stderr io.Writer) error {
	ui := newPlaygroundTerminal(stdin, stdout, stderr)
	acpCmd := buildGeminiACPCommand(opts)
	tracef := func(format string, args ...any) {
		if !opts.DebugEvents {
			return
		}
		ui.PrintfErrf("[acp] "+format+"\n", args...)
	}

	agentRuntime, err := acpagent.New(acpagent.Config{
		Context:           ctx,
		Name:              "GeminiACP",
		Description:       "Gemini CLI playground agent via ACP",
		Command:           acpCmd,
		WorkingDir:        repoRoot,
		Stderr:            io.Discard,
		PermissionHandler: ui.RequestPermission,
		Tracef:            tracef,
	})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := agentRuntime.Close(); closeErr != nil {
			log.Debug().Err(closeErr).Msg("close gemini acp agent")
		}
	}()

	runner, sess, err := newPlaygroundRunner(ctx, agentRuntime)
	if err != nil {
		return err
	}

	if strings.TrimSpace(opts.Prompt) != "" {
		return runPlaygroundTurn(ctx, runner, sess, ui, opts.Prompt)
	}

	for {
		line, err := ui.ReadLine("> ")
		if errors.Is(err, io.EOF) {
			ui.Println()
			return nil
		}
		if err != nil {
			return err
		}
		prompt := strings.TrimSpace(line)
		if prompt == "" {
			continue
		}
		switch prompt {
		case "exit", "quit":
			return nil
		}
		if err := runPlaygroundTurn(ctx, runner, sess, ui, prompt); err != nil {
			return err
		}
	}
}

func buildGeminiACPCommand(opts geminiACPOptions) []string {
	cmd := []string{opts.GeminiBin, "--experimental-acp"}
	if strings.TrimSpace(opts.Model) != "" {
		cmd = append(cmd, "--model", opts.Model)
	}
	cmd = append(cmd, opts.GeminiArgs...)
	return cmd
}

func newPlaygroundRunner(ctx context.Context, a agent.Agent) (*runnerpkg.Runner, session.Session, error) {
	sessionService := session.InMemoryService()
	r, err := runnerpkg.New(runnerpkg.Config{
		AppName:        "norma-playground",
		Agent:          a,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create playground runner: %w", err)
	}
	sess, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName: "norma-playground",
		UserID:  "norma-playground-user",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create playground session: %w", err)
	}
	return r, sess.Session, nil
}

func runPlaygroundTurn(ctx context.Context, r *runnerpkg.Runner, sess session.Session, ui *playgroundTerminal, prompt string) error {
	events := r.Run(ctx, "norma-playground-user", sess.ID(), genai.NewContentFromText(prompt, genai.RoleUser), agent.RunConfig{})
	printedPartial := false
	printedFinal := false
	for ev, err := range events {
		if err != nil {
			return err
		}
		text := extractEventText(ev)
		if text == "" {
			continue
		}
		if ev.Partial {
			printedPartial = true
			ui.Print(text)
			continue
		}
		if printedPartial {
			continue
		}
		printedFinal = true
		ui.Println(text)
	}
	if printedPartial && !printedFinal {
		ui.Println()
	}
	return nil
}

func extractEventText(ev *session.Event) string {
	if ev == nil || ev.Content == nil {
		return ""
	}
	var builder strings.Builder
	for _, part := range ev.Content.Parts {
		if part == nil || part.Text == "" {
			continue
		}
		builder.WriteString(part.Text)
	}
	return builder.String()
}

type playgroundTerminal struct {
	reader *bufio.Reader
	stdout io.Writer
	stderr io.Writer
	mu     sync.Mutex
}

func newPlaygroundTerminal(stdin io.Reader, stdout, stderr io.Writer) *playgroundTerminal {
	return &playgroundTerminal{
		reader: bufio.NewReader(stdin),
		stdout: stdout,
		stderr: stderr,
	}
}

func (t *playgroundTerminal) ReadLine(prompt string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if prompt != "" {
		if _, err := fmt.Fprint(t.stdout, prompt); err != nil {
			return "", err
		}
	}
	line, err := t.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if errors.Is(err, io.EOF) && len(line) == 0 {
		return "", io.EOF
	}
	return line, nil
}

func (t *playgroundTerminal) Print(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = io.WriteString(t.stdout, text)
}

func (t *playgroundTerminal) Println(args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintln(t.stdout, args...)
}

func (t *playgroundTerminal) PrintfErrf(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintf(t.stderr, format, args...)
}

func (t *playgroundTerminal) RequestPermission(_ context.Context, req acpagent.RequestPermissionRequest) (acpagent.RequestPermissionResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, err := fmt.Fprintf(t.stdout, "\nPermission requested: %s\n", req.ToolCall.Title); err != nil {
		return acpagent.RequestPermissionResponse{}, err
	}
	for idx, option := range req.Options {
		if _, err := fmt.Fprintf(t.stdout, "%d. %s (%s)\n", idx+1, option.Name, option.Kind); err != nil {
			return acpagent.RequestPermissionResponse{}, err
		}
	}
	if _, err := fmt.Fprint(t.stdout, "Choose an option: "); err != nil {
		return acpagent.RequestPermissionResponse{}, err
	}

	line, err := t.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return acpagent.RequestPermissionResponse{}, err
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return acpagent.RequestPermissionResponse{Outcome: acpagent.PermissionOutcome{Outcome: "cancelled"}}, nil
	}

	choice, convErr := strconv.Atoi(trimmed)
	if convErr != nil || choice < 1 || choice > len(req.Options) {
		return acpagent.RequestPermissionResponse{Outcome: acpagent.PermissionOutcome{Outcome: "cancelled"}}, nil
	}
	selected := req.Options[choice-1]
	return acpagent.RequestPermissionResponse{Outcome: acpagent.PermissionOutcome{Outcome: "selected", OptionID: selected.OptionID}}, nil
}
