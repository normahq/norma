package planner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/metalagman/norma/internal/adk/agentfactory"
	"github.com/metalagman/norma/internal/config"
	domain "github.com/metalagman/norma/internal/planner"
	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// AgentPlanner runs interactive planner conversations using configured agent runtimes.
type AgentPlanner struct {
	repoRoot  string
	registry  map[string]config.AgentConfig
	plannerID string
}

// NewAgentPlanner constructs a new planner runtime.
func NewAgentPlanner(repoRoot string, registry map[string]config.AgentConfig, plannerID string) *AgentPlanner {
	return &AgentPlanner{
		repoRoot:  repoRoot,
		registry:  registry,
		plannerID: plannerID,
	}
}

// RunInteractive starts a planner session in TUI mode.
func (p *AgentPlanner) RunInteractive(ctx context.Context, req domain.Request) (string, error) {
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
		if runErr := RunTUI(prog); runErr != nil {
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
	stopTUI := func() {
		closeEvents()
		prog.Quit()
	}

	factory := agentfactory.NewFactory(p.registry)

	creationReq := agentfactory.CreationRequest{
		Name:              "NormaPlannerAgent",
		Description:       "Norma planner via generic agent runtime",
		SystemInstruction: PlannerInstruction(),
		WorkingDirectory:  p.repoRoot,
		Stderr:            io.Discard,
	}

	agentRuntime, err := factory.CreateAgent(runCtx, p.plannerID, creationReq)
	if err != nil {
		stopTUI()
		_ = waitTUI()
		return "", fmt.Errorf("create planner runtime: %w", err)
	}
	if closer, ok := agentRuntime.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}

	sessionService := session.InMemoryService()
	adkRunner, err := runner.New(runner.Config{
		AppName:        "norma-plan-agent",
		Agent:          agentRuntime,
		SessionService: sessionService,
	})
	if err != nil {
		stopTUI()
		_ = waitTUI()
		return "", fmt.Errorf("create planner runner: %w", err)
	}

	sess, err := sessionService.Create(runCtx, &session.CreateRequest{
		AppName: "norma-plan-agent",
		UserID:  "norma-planner-user",
	})
	if err != nil {
		stopTUI()
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

	if initialPrompt := buildInitialPrompt(req); initialPrompt != "" {
		if turnErr := runTurn(initialPrompt); turnErr != nil {
			if errors.Is(turnErr, context.Canceled) {
				stopTUI()
				_ = waitTUI()
				return "", context.Canceled
			}
			prog.Send(planFailedMsg(formatPlannerRunError(turnErr)))
			if tuiErr := waitTUI(); tuiErr != nil {
				return "", fmt.Errorf("TUI error: %w", tuiErr)
			}
			closeEvents()
			return "", ErrHandledInTUI
		}
	}

	for {
		select {
		case questionChan <- "What do you want to build? Ctrl+C to exit.":
		case <-runCtx.Done():
			stopTUI()
			_ = waitTUI()
			return "", context.Canceled
		}

		var input string
		select {
		case input = <-responseChan:
		case <-runCtx.Done():
			stopTUI()
			_ = waitTUI()
			return "", context.Canceled
		}

		message := strings.TrimSpace(input)
		if message == "" {
			continue
		}
		switch strings.ToLower(message) {
		case "exit", "quit":
			prog.Send(planCompletedMsg("Planner session ended by user."))
			if tuiErr := waitTUI(); tuiErr != nil {
				return "", fmt.Errorf("TUI error: %w", tuiErr)
			}
			closeEvents()
			return planRunDir, nil
		}

		if turnErr := runTurn(message); turnErr != nil {
			if errors.Is(turnErr, context.Canceled) {
				stopTUI()
				_ = waitTUI()
				return "", context.Canceled
			}
			prog.Send(planFailedMsg(formatPlannerRunError(turnErr)))
			if tuiErr := waitTUI(); tuiErr != nil {
				return "", fmt.Errorf("TUI error: %w", tuiErr)
			}
			closeEvents()
			return "", ErrHandledInTUI
		}
	}
}

func buildInitialPrompt(req domain.Request) string {
	var b strings.Builder

	epicDescription := strings.TrimSpace(req.EpicDescription)
	if epicDescription != "" {
		b.WriteString(epicDescription)
	}

	for _, c := range req.Clarifications {
		question := strings.TrimSpace(c.Question)
		answer := strings.TrimSpace(c.Answer)
		if question == "" && answer == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		if question != "" {
			b.WriteString("Clarification: ")
			b.WriteString(question)
			if answer != "" {
				b.WriteString("\nAnswer: ")
				b.WriteString(answer)
			}
			continue
		}
		b.WriteString("Clarification answer: ")
		b.WriteString(answer)
	}

	return b.String()
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

func randomHex(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
