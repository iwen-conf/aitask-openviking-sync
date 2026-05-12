package tasks

import (
	"errors"
	"fmt"
)

type Status string

const (
	StatusPlanned   Status = "planned"
	StatusDelegated Status = "delegated"
	StatusRunning   Status = "running"
	StatusSubmitted Status = "submitted"
	StatusReviewing Status = "reviewing"
	StatusDone      Status = "done"
	StatusBlocked   Status = "blocked"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

var (
	ErrTaskNotDelegated       = errors.New("task is not delegated")
	ErrTaskNotAssignedToAgent = errors.New("task is not assigned to current agent")
	ErrTaskActiveRunMismatch  = errors.New("task active run does not match current run")
	ErrTaskStatusInvalid      = errors.New("task status does not allow this operation")
	ErrTaskDependencyNotDone  = errors.New("task dependencies are not done")
	ErrTaskMissingAssignee    = errors.New("task delegation requires an assignee")
	ErrTaskMissingActiveRun   = errors.New("task start requires an active run")
	ErrTaskMissingDelegatedBy = errors.New("task delegation requires a delegator")
)

type Task struct {
	ID                string
	ProjectID         string
	Status            Status
	AssigneeAgentID   string
	AssigneeAgentType string
	DelegatedByType   string
	DelegatedByID     string
	ActiveRunID       string
	DependenciesDone  bool
}

type DelegateCommand struct {
	AssigneeAgentID   string
	AssigneeAgentType string
	DelegatedByType   string
	DelegatedByID     string
}

func (t Task) Delegate(cmd DelegateCommand) (Task, error) {
	if cmd.AssigneeAgentID == "" {
		return Task{}, ErrTaskMissingAssignee
	}
	if cmd.DelegatedByType == "" {
		return Task{}, ErrTaskMissingDelegatedBy
	}
	if t.Status != StatusPlanned && t.Status != StatusDelegated && t.Status != StatusBlocked {
		return Task{}, fmt.Errorf("%w: cannot delegate from %s", ErrTaskStatusInvalid, t.Status)
	}

	t.Status = StatusDelegated
	t.AssigneeAgentID = cmd.AssigneeAgentID
	t.AssigneeAgentType = cmd.AssigneeAgentType
	t.DelegatedByType = cmd.DelegatedByType
	t.DelegatedByID = cmd.DelegatedByID
	t.ActiveRunID = ""
	return t, nil
}

func (t Task) Start(agentID string, runID string) (Task, error) {
	if t.Status != StatusDelegated {
		return Task{}, ErrTaskNotDelegated
	}
	if t.AssigneeAgentID == "" {
		return Task{}, ErrTaskMissingAssignee
	}
	if t.AssigneeAgentID != agentID {
		return Task{}, ErrTaskNotAssignedToAgent
	}
	if runID == "" {
		return Task{}, ErrTaskMissingActiveRun
	}
	if !t.DependenciesDone {
		return Task{}, ErrTaskDependencyNotDone
	}

	t.Status = StatusRunning
	t.ActiveRunID = runID
	return t, nil
}

func (t Task) Submit(agentID string, runID string) (Task, error) {
	if t.Status != StatusRunning {
		return Task{}, fmt.Errorf("%w: cannot submit from %s", ErrTaskStatusInvalid, t.Status)
	}
	if t.AssigneeAgentID != agentID {
		return Task{}, ErrTaskNotAssignedToAgent
	}
	if t.ActiveRunID != runID {
		return Task{}, ErrTaskActiveRunMismatch
	}

	t.Status = StatusSubmitted
	return t, nil
}
