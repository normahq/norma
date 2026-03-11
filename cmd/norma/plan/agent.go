package plancmd

import (
	"context"
	"errors"
	"fmt"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/metalagman/norma/internal/agents/planner"
	"github.com/metalagman/norma/internal/config"
	domain "github.com/metalagman/norma/internal/planner"
	"github.com/spf13/cobra"
)

func runAgentPlanner(cmd *cobra.Command, repoRoot string, registry map[string]config.AgentConfig, plannerID string, req domain.Request) error {
	p := planner.NewAgentPlanner(repoRoot, registry, plannerID)
	sess, err := p.StartInteractive(cmd.Context(), req)
	if err != nil {
		return err
	}

	tuiModel, err := newPlannerModel(sess.Events, sess.Questions, sess.Responses, sess.Cancel)
	if err != nil {
		sess.Cancel()
		_ = sess.Wait()
		return fmt.Errorf("create TUI model: %w", err)
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

	runErr := sess.Wait()
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) {
			if tuiErr := waitTUI(); tuiErr != nil {
				return fmt.Errorf("TUI error: %w", tuiErr)
			}
			return nil
		}
		prog.Send(planFailedMsg(planner.FormatPlannerRunError(runErr)))
		if tuiErr := waitTUI(); tuiErr != nil {
			return fmt.Errorf("TUI error: %w", tuiErr)
		}
		return nil
	}

	prog.Send(planCompletedMsg("Planner session ended by user."))
	if tuiErr := waitTUI(); tuiErr != nil {
		return fmt.Errorf("TUI error: %w", tuiErr)
	}

	fmt.Printf("\nPlanner session complete.\n")
	fmt.Printf("Planning run directory: %s\n", sess.RunDir)
	return nil
}
