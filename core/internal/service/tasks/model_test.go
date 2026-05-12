package tasks

import (
	"errors"
	"testing"
)

func TestDelegateRequiresConcreteAssignee(t *testing.T) {
	task := Task{Status: StatusPlanned}

	_, err := task.Delegate(DelegateCommand{
		DelegatedByType: "operator",
	})

	if !errors.Is(err, ErrTaskMissingAssignee) {
		t.Fatalf("Delegate() error = %v, want %v", err, ErrTaskMissingAssignee)
	}
}

func TestDelegatedTaskCanStartOnlyByAssignee(t *testing.T) {
	task := Task{
		Status:           StatusDelegated,
		AssigneeAgentID:  "agt_codex_1",
		DependenciesDone: true,
	}

	_, err := task.Start("agt_gemini_1", "run_1")
	if !errors.Is(err, ErrTaskNotAssignedToAgent) {
		t.Fatalf("Start() error = %v, want %v", err, ErrTaskNotAssignedToAgent)
	}

	started, err := task.Start("agt_codex_1", "run_1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.Status != StatusRunning {
		t.Fatalf("started.Status = %q, want %q", started.Status, StatusRunning)
	}
	if started.ActiveRunID != "run_1" {
		t.Fatalf("started.ActiveRunID = %q, want run_1", started.ActiveRunID)
	}
}

func TestSubmitRequiresMatchingActiveRun(t *testing.T) {
	task := Task{
		Status:          StatusRunning,
		AssigneeAgentID: "agt_codex_1",
		ActiveRunID:     "run_1",
	}

	_, err := task.Submit("agt_codex_1", "run_2")
	if !errors.Is(err, ErrTaskActiveRunMismatch) {
		t.Fatalf("Submit() error = %v, want %v", err, ErrTaskActiveRunMismatch)
	}

	submitted, err := task.Submit("agt_codex_1", "run_1")
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if submitted.Status != StatusSubmitted {
		t.Fatalf("submitted.Status = %q, want %q", submitted.Status, StatusSubmitted)
	}
}
