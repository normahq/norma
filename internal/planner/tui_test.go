package planner

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/adk/session"
)

func TestPlannerModel_SendMessageStartsSpinner(t *testing.T) {
	t.Parallel()

	eventChan := make(chan *session.Event)
	questionChan := make(chan string)
	responseChan := make(chan string, 1)

	model, err := newPlannerModel(eventChan, questionChan, responseChan, nil)
	if err != nil {
		t.Fatalf("newPlannerModel() error = %v", err)
	}

	next, _ := model.Update(humanRequestMsg("What should we build?"))
	pm, ok := next.(*plannerModel)
	if !ok {
		t.Fatalf("Update() model type = %T, want *plannerModel", next)
	}
	pm.textinput.SetValue("Build a TUI spinner.")

	next, cmd := pm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Update(Enter) returned nil cmd, want spinner tick cmd")
	}

	pm, ok = next.(*plannerModel)
	if !ok {
		t.Fatalf("Update(Enter) model type = %T, want *plannerModel", next)
	}
	if pm.waitingForHuman {
		t.Fatal("waitingForHuman = true, want false")
	}
	if !pm.waitingResponse {
		t.Fatal("waitingResponse = false, want true")
	}
	if got := pm.status; got != "Message sent. Waiting for planner response..." {
		t.Fatalf("status = %q, want %q", got, "Message sent. Waiting for planner response...")
	}

	select {
	case got := <-responseChan:
		if got != "Build a TUI spinner." {
			t.Fatalf("response = %q, want %q", got, "Build a TUI spinner.")
		}
	default:
		t.Fatal("response channel did not receive sent message")
	}
}

func TestPlannerModel_EventResponseStopsSpinner(t *testing.T) {
	t.Parallel()

	eventChan := make(chan *session.Event)
	questionChan := make(chan string)
	responseChan := make(chan string, 1)

	model, err := newPlannerModel(eventChan, questionChan, responseChan, nil)
	if err != nil {
		t.Fatalf("newPlannerModel() error = %v", err)
	}
	model.waitingResponse = true

	next, _ := model.Update(eventMsg(&session.Event{}))
	pm, ok := next.(*plannerModel)
	if !ok {
		t.Fatalf("Update(event) model type = %T, want *plannerModel", next)
	}
	if pm.waitingResponse {
		t.Fatal("waitingResponse = true, want false")
	}
}

func TestPlannerModel_HumanPromptStopsSpinner(t *testing.T) {
	t.Parallel()

	eventChan := make(chan *session.Event)
	questionChan := make(chan string)
	responseChan := make(chan string, 1)

	model, err := newPlannerModel(eventChan, questionChan, responseChan, nil)
	if err != nil {
		t.Fatalf("newPlannerModel() error = %v", err)
	}
	model.waitingResponse = true

	next, _ := model.Update(humanRequestMsg("Please clarify target user."))
	pm, ok := next.(*plannerModel)
	if !ok {
		t.Fatalf("Update(humanRequestMsg) model type = %T, want *plannerModel", next)
	}
	if pm.waitingResponse {
		t.Fatal("waitingResponse = true, want false")
	}
	if !pm.waitingForHuman {
		t.Fatal("waitingForHuman = false, want true")
	}
}
