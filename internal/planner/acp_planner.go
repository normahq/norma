package planner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	acp "github.com/coder/acp-go-sdk"
	"github.com/metalagman/norma/internal/adk/acpagent"
	normaagent "github.com/metalagman/norma/internal/agent"
	"github.com/metalagman/norma/internal/config"
	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// ACPPlanner runs interactive planner conversations using ACP-backed models.
type ACPPlanner struct {
	repoRoot string
	cfg      config.AgentConfig
}

// NewACPPlanner constructs a new ACP planner runtime.
func NewACPPlanner(repoRoot string, cfg config.AgentConfig) *ACPPlanner {
	return &ACPPlanner{
		repoRoot: repoRoot,
		cfg:      cfg,
	}
}

// RunInteractive starts an ACP planner session in TUI mode.
func (p *ACPPlanner) RunInteractive(ctx context.Context, req Request) (string, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	planRunDir, err := newPlanRunDir(p.repoRoot)
	if err != nil {
		return "", err
	}

	eventChan := make(chan *session.Event, 100)
	questionChan := make(chan string)
	responseChan := make(chan string)

	tuiModel, err := newPlannerModel(eventChan, questionChan, responseChan, cancel)
	if err != nil {
		return "", fmt.Errorf("create TUI model: %w", err)
	}
	prog := tea.NewProgram(tuiModel, tea.WithAltScreen())

	tuiErrChan := make(chan error, 1)
	go func() {
		if _, runErr := prog.Run(); runErr != nil {
			tuiErrChan <- runErr
		}
		close(tuiErrChan)
	}()

	var waitTUIOnce sync.Once
	var waitTUIErr error
	waitTUI := func() error {
		waitTUIOnce.Do(func() {
			if runErr, ok := <-tuiErrChan; ok {
				waitTUIErr = runErr
			}
		})
		return waitTUIErr
	}

	var closeEventOnce sync.Once
	closeEvents := func() {
		closeEventOnce.Do(func() {
			close(eventChan)
			close(questionChan)
		})
	}

	acpCmd, err := normaagent.ResolveACPCommand(p.cfg)
	if err != nil {
		closeEvents()
		_ = waitTUI()
		return "", err
	}

	acpRuntime, err := acpagent.New(acpagent.Config{
		Context:           runCtx,
		Name:              "NormaPlannerACP",
		Description:       "Norma planner via ACP runtime",
		Command:           acpCmd,
		WorkingDir:        p.repoRoot,
		Stderr:            io.Discard,
		PermissionHandler: allowACPPermissions,
	})
	if err != nil {
		closeEvents()
		_ = waitTUI()
		return "", fmt.Errorf("create ACP planner runtime: %w", err)
	}
	defer func() { _ = acpRuntime.Close() }()

	sessionService := session.InMemoryService()
	adkRunner, err := runner.New(runner.Config{
		AppName:        "norma-plan-acp",
		Agent:          acpRuntime,
		SessionService: sessionService,
	})
	if err != nil {
		closeEvents()
		_ = waitTUI()
		return "", fmt.Errorf("create planner runner: %w", err)
	}

	sess, err := sessionService.Create(runCtx, &session.CreateRequest{
		AppName: "norma-plan-acp",
		UserID:  "norma-planner-user",
	})
	if err != nil {
		closeEvents()
		_ = waitTUI()
		return "", fmt.Errorf("create planner session: %w", err)
	}

	runTurn := func(prompt string) error {
		events := adkRunner.Run(
			runCtx,
			"norma-planner-user",
			sess.Session.ID(),
			genai.NewContentFromText(prompt, genai.RoleUser),
			adkagent.RunConfig{},
		)
		for ev, runErr := range events {
			if runErr != nil {
				return fmt.Errorf("planner turn failed: %w", runErr)
			}
			if ev == nil {
				continue
			}
			select {
			case eventChan <- ev:
			case <-runCtx.Done():
				return runCtx.Err()
			}
		}
		return nil
	}

	if goal := strings.TrimSpace(req.EpicDescription); goal != "" {
		initialPrompt := strings.TrimSpace(fmt.Sprintf(
			"You are Norma planner. Create and refine a project plan conversationally. JSON formatting is not required. "+
				"If needed, run beads and shell commands directly through your ACP runtime.\n\nProject goal:\n%s",
			goal,
		))
		if turnErr := runTurn(initialPrompt); turnErr != nil {
			if errors.Is(turnErr, context.Canceled) {
				closeEvents()
				_ = waitTUI()
				return "", context.Canceled
			}
			closeEvents()
			prog.Send(planFailedMsg(formatPlannerRunError(turnErr)))
			if tuiErr := waitTUI(); tuiErr != nil {
				return "", fmt.Errorf("TUI error: %w", tuiErr)
			}
			return "", ErrHandledInTUI
		}
	}

	for {
		select {
		case questionChan <- "Enter planner message (`exit` to finish):":
		case <-runCtx.Done():
			closeEvents()
			_ = waitTUI()
			return "", context.Canceled
		}

		var input string
		select {
		case input = <-responseChan:
		case <-runCtx.Done():
			closeEvents()
			_ = waitTUI()
			return "", context.Canceled
		}

		message := strings.TrimSpace(input)
		if message == "" {
			continue
		}
		switch strings.ToLower(message) {
		case "exit", "quit":
			closeEvents()
			prog.Send(planCompletedMsg("Planner session ended by user."))
			if tuiErr := waitTUI(); tuiErr != nil {
				return "", fmt.Errorf("TUI error: %w", tuiErr)
			}
			return planRunDir, nil
		}

		if turnErr := runTurn(message); turnErr != nil {
			if errors.Is(turnErr, context.Canceled) {
				closeEvents()
				_ = waitTUI()
				return "", context.Canceled
			}
			closeEvents()
			prog.Send(planFailedMsg(formatPlannerRunError(turnErr)))
			if tuiErr := waitTUI(); tuiErr != nil {
				return "", fmt.Errorf("TUI error: %w", tuiErr)
			}
			return "", ErrHandledInTUI
		}
	}
}

func newPlanRunDir(repoRoot string) (string, error) {
	sfx, err := randomHex(3)
	if err != nil {
		return "", fmt.Errorf("generate planning run id: %w", err)
	}
	runID := fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405"), sfx)
	runDir := filepath.Join(repoRoot, ".norma", "plans", runID)
	if err := os.MkdirAll(filepath.Join(runDir, "logs"), 0o700); err != nil {
		return "", fmt.Errorf("create planning logs dir: %w", err)
	}
	return runDir, nil
}

func allowACPPermissions(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	for _, option := range req.Options {
		if option.Kind == acp.PermissionOptionKindAllowOnce || option.Kind == acp.PermissionOptionKindAllowAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId),
			}, nil
		}
	}
	for _, option := range req.Options {
		if option.Kind == acp.PermissionOptionKindRejectOnce || option.Kind == acp.PermissionOptionKindRejectAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId),
			}, nil
		}
	}
	return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
}
