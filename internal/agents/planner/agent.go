package planner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// InteractiveSession exposes the planner runtime interaction channels.
type InteractiveSession struct {
	RunDir    string
	Events    <-chan *session.Event
	Questions <-chan string
	Responses chan<- string

	done chan error

	cancel   context.CancelFunc
	waitOnce sync.Once
	waitErr  error
}

// Wait blocks until the interactive planner session ends.
func (s *InteractiveSession) Wait() error {
	s.waitOnce.Do(func() {
		s.waitErr = <-s.done
	})
	return s.waitErr
}

// Cancel stops the interactive planner session.
func (s *InteractiveSession) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// NewAgentPlanner constructs a new planner runtime.
func NewAgentPlanner(repoRoot string, registry map[string]config.AgentConfig, plannerID string) *AgentPlanner {
	return &AgentPlanner{
		repoRoot:  repoRoot,
		registry:  registry,
		plannerID: plannerID,
	}
}

// StartInteractive starts an interactive planner runtime and returns session I/O channels.
func (p *AgentPlanner) StartInteractive(ctx context.Context, req domain.Request) (*InteractiveSession, error) {
	runCtx, cancel := context.WithCancel(ctx)

	planRunDir, err := newPlanRunDir(p.repoRoot)
	if err != nil {
		cancel()
		return nil, err
	}

	eventChan := make(chan *session.Event, 100)
	questionChan := make(chan string)
	responseChan := make(chan string)
	done := make(chan error, 1)

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
		cancel()
		return nil, fmt.Errorf("create planner runtime: %w", err)
	}

	sessionService := session.InMemoryService()
	adkRunner, err := runner.New(runner.Config{
		AppName:        "norma-plan-agent",
		Agent:          agentRuntime,
		SessionService: sessionService,
	})
	if err != nil {
		if closer, ok := agentRuntime.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
		cancel()
		return nil, fmt.Errorf("create planner runner: %w", err)
	}

	sess, err := sessionService.Create(runCtx, &session.CreateRequest{
		AppName: "norma-plan-agent",
		UserID:  "norma-planner-user",
	})
	if err != nil {
		if closer, ok := agentRuntime.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
		cancel()
		return nil, fmt.Errorf("create planner session: %w", err)
	}

	go func() {
		defer close(eventChan)
		defer close(questionChan)
		defer close(done)
		defer cancel()
		if closer, ok := agentRuntime.(interface{ Close() error }); ok {
			defer func() { _ = closer.Close() }()
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
				done <- turnErr
				return
			}
		}

		for {
			select {
			case questionChan <- "What do you want to build? Ctrl+C to exit.":
			case <-runCtx.Done():
				done <- runCtx.Err()
				return
			}

			var input string
			select {
			case input = <-responseChan:
			case <-runCtx.Done():
				done <- runCtx.Err()
				return
			}

			message := strings.TrimSpace(input)
			if message == "" {
				continue
			}
			switch strings.ToLower(message) {
			case "exit", "quit":
				done <- nil
				return
			}

			if turnErr := runTurn(message); turnErr != nil {
				done <- turnErr
				return
			}
		}
	}()

	return &InteractiveSession{
		RunDir:    planRunDir,
		Events:    eventChan,
		Questions: questionChan,
		Responses: responseChan,
		done:      done,
		cancel:    cancel,
	}, nil
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
