package tasks

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
)

func TestEnsureTransitionAllowed(t *testing.T) {
	if err := ensureTransitionAllowed(StatusPlanned, StatusDelegated); err != nil {
		t.Fatalf("ensureTransitionAllowed planned->delegated error = %v", err)
	}
	if err := ensureTransitionAllowed(StatusDone, StatusDelegated); !errors.Is(err, ErrTaskStatusInvalid) {
		t.Fatalf("ensureTransitionAllowed done->delegated error = %v, want %v", err, ErrTaskStatusInvalid)
	}
}

func TestGetTaskCrossProjectIsolation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks").WithArgs("prj_b", "task_1").WillReturnError(sql.ErrNoRows)

	_, err = svc.Get(context.Background(), "prj_b", "task_1")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("Get() error = %v, want %v", err, ErrTaskNotFound)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestStartRejectsCrossProjectTaskLookup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectQuery("SELECT status FROM projects WHERE id = \\$1").WithArgs("prj_b").WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks").WithArgs("prj_b", "task_1").WillReturnError(sql.ErrNoRows)

	_, err = svc.Start(context.Background(), identity.Identity{SenderType: identity.SenderTypeAgent, Agent: identity.AgentIdentity{AgentID: "agt_1"}}, "prj_b", "task_1", StartTaskInput{RunID: "run_1"})
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("Start() error = %v, want %v", err, ErrTaskNotFound)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestScanTaskRowDefaultsDelegatedByType(t *testing.T) {
	rows := sqlmock.NewRows([]string{
		"id", "project_id", "title", "description", "goal", "input_context", "constraints_text", "status", "parent_task_id",
		"assignee_agent_id", "assignee_agent_type", "delegated_by_type", "delegated_by_operator_label",
		"delegated_by_agent_id", "delegated_at", "active_run_id", "last_heartbeat_at",
		"required_model", "output_contract", "priority", "created_at", "updated_at",
	}).AddRow(
		"task_1", "prj_1", "title", "desc", nil, nil, nil, "planned", nil,
		nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil, 0, time.Now().UTC(), time.Now().UTC(),
	)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT 1").WillReturnRows(rows)
	row := db.QueryRow("SELECT 1")

	task, err := scanTaskRow(row)
	if err != nil {
		t.Fatalf("scanTaskRow() error = %v", err)
	}
	if got, want := task.DelegatedByType, "system"; got != want {
		t.Fatalf("DelegatedByType = %q, want %q", got, want)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestDelegateAcceptsEmptyAgentType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	now := time.Now().UTC()
	mock.ExpectQuery("SELECT status FROM projects WHERE id = \\$1").WithArgs("prj_1").WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks[\\s\\S]*WHERE project_id = \\$1 AND id = \\$2").
		WithArgs("prj_1", "task_1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "title", "description", "goal", "input_context", "constraints_text", "status", "parent_task_id",
			"assignee_agent_id", "assignee_agent_type", "delegated_by_type", "delegated_by_operator_label",
			"delegated_by_agent_id", "delegated_at", "active_run_id", "last_heartbeat_at",
			"required_model", "output_contract", "priority", "created_at", "updated_at",
		}).AddRow(
			"task_1", "prj_1", "title", "desc", nil, nil, nil, "planned", nil,
			nil, nil, "operator", "local-operator",
			nil, nil, nil, nil,
			nil, nil, 1, now, now,
		))
	mock.ExpectQuery("SELECT depends_on_task_id FROM task_dependencies").WithArgs("task_1").WillReturnRows(sqlmock.NewRows([]string{"depends_on_task_id"}))
	mock.ExpectQuery("SELECT skill_name FROM task_required_skills").WithArgs("task_1").WillReturnRows(sqlmock.NewRows([]string{"skill_name"}))

	mock.ExpectQuery("SELECT agent_type, status FROM agents WHERE id = \\$1").
		WithArgs("agt_1").
		WillReturnRows(sqlmock.NewRows([]string{"agent_type", "status"}).AddRow("codex", "active"))
	mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM agent_project_bindings").WithArgs("agt_1", "prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("UPDATE tasks").
		WithArgs("prj_1", "task_1", "agt_1", "codex", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO task_delegations").
		WithArgs(sqlmock.AnyArg(), "task_1", "agt_1", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO task_events").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks[\\s\\S]*WHERE project_id = \\$1 AND id = \\$2").
		WithArgs("prj_1", "task_1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "title", "description", "goal", "input_context", "constraints_text", "status", "parent_task_id",
			"assignee_agent_id", "assignee_agent_type", "delegated_by_type", "delegated_by_operator_label",
			"delegated_by_agent_id", "delegated_at", "active_run_id", "last_heartbeat_at",
			"required_model", "output_contract", "priority", "created_at", "updated_at",
		}).AddRow(
			"task_1", "prj_1", "title", "desc", nil, nil, nil, "delegated", nil,
			"agt_1", "codex", "operator", "local-operator",
			nil, now, nil, nil,
			nil, nil, 1, now, now,
		))
	mock.ExpectQuery("SELECT depends_on_task_id FROM task_dependencies").WithArgs("task_1").WillReturnRows(sqlmock.NewRows([]string{"depends_on_task_id"}))
	mock.ExpectQuery("SELECT skill_name FROM task_required_skills").WithArgs("task_1").WillReturnRows(sqlmock.NewRows([]string{"skill_name"}))

	_, err = svc.Delegate(
		context.Background(),
		identity.Operator("local-operator"),
		"prj_1",
		"task_1",
		DelegateTaskInput{AgentID: "agt_1", AgentType: "", Reason: "delegate"},
	)
	if err != nil {
		t.Fatalf("Delegate() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestRunContextGateBlocksHandoffOnlySubmit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	now := time.Now().UTC()
	mock.ExpectQuery("SELECT status FROM projects WHERE id = \\$1").WithArgs("prj_1").WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks[\\s\\S]*WHERE project_id = \\$1 AND id = \\$2").
		WithArgs("prj_1", "task_1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "title", "description", "goal", "input_context", "constraints_text", "status", "parent_task_id",
			"assignee_agent_id", "assignee_agent_type", "delegated_by_type", "delegated_by_operator_label",
			"delegated_by_agent_id", "delegated_at", "active_run_id", "last_heartbeat_at",
			"required_model", "output_contract", "priority", "created_at", "updated_at",
		}).AddRow(
			"task_1", "prj_1", "title", "desc", nil, nil, nil, "running", nil,
			"agt_1", "codex", "operator", "local-operator",
			nil, now, "run_1", now,
			nil, nil, 1, now, now,
		))
	mock.ExpectQuery("SELECT depends_on_task_id FROM task_dependencies").WithArgs("task_1").WillReturnRows(sqlmock.NewRows([]string{"depends_on_task_id"}))
	mock.ExpectQuery("SELECT skill_name FROM task_required_skills").WithArgs("task_1").WillReturnRows(sqlmock.NewRows([]string{"skill_name"}))
	mock.ExpectQuery("SELECT context_state FROM agent_runs WHERE id = \\$1").
		WithArgs("run_1").
		WillReturnRows(sqlmock.NewRows([]string{"context_state"}).AddRow("handoff_only"))

	_, err = svc.Submit(
		context.Background(),
		identity.Identity{SenderType: identity.SenderTypeAgent, Agent: identity.AgentIdentity{AgentID: "agt_1"}},
		"prj_1",
		"task_1",
		SubmitTaskInput{RunID: "run_1", ResultMarkdown: "x"},
	)
	if !errors.Is(err, ErrContextHandoffRequired) {
		t.Fatalf("Submit() error = %v, want ErrContextHandoffRequired", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestResumeAllowsHeartbeatTimeoutBlockedTaskWithoutHandoff(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	now := time.Now().UTC()
	actor := identity.Identity{SenderType: identity.SenderTypeAgent, Agent: identity.AgentIdentity{AgentID: "agt_1"}}

	mock.ExpectQuery("SELECT status FROM projects WHERE id = \\$1").
		WithArgs("prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	expectTaskGet(mock, "prj_1", "task_1", "blocked", "agt_1", nil, now)
	mock.ExpectQuery("SELECT COALESCE\\(error, ''\\) FROM tasks").
		WithArgs("prj_1", "task_1").
		WillReturnRows(sqlmock.NewRows([]string{"error"}).AddRow("active_run_heartbeat_timeout"))
	mock.ExpectQuery("SELECT COUNT\\(1\\)[\\s\\S]*FROM task_dependencies").
		WithArgs("task_1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT agent_id, project_id FROM agent_runs WHERE id = \\$1").
		WithArgs("run_recover_1").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO agent_runs").
		WithArgs("run_recover_1", "agt_1", "prj_1", "task_1", nil, 200000).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT context_state FROM agent_runs WHERE id = \\$1").
		WithArgs("run_recover_1").
		WillReturnRows(sqlmock.NewRows([]string{"context_state"}).AddRow("normal"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks[\\s\\S]*SET status = 'running'").
		WithArgs("prj_1", "task_1", "run_recover_1", "agt_1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT session_id FROM tasks WHERE id = \\$1 AND project_id = \\$2").
		WithArgs("task_1", "prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"session_id"}).AddRow("sess_1"))
	mock.ExpectExec("INSERT INTO task_events").
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectTaskGet(mock, "prj_1", "task_1", "running", "agt_1", stringPtr("run_recover_1"), now)

	task, err := svc.Resume(
		context.Background(),
		actor,
		"prj_1",
		"task_1",
		ResumeTaskInput{RunID: "run_recover_1"},
	)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if task.Status != StatusRunning {
		t.Fatalf("status = %s, want %s", task.Status, StatusRunning)
	}
	if task.ActiveRunID == nil || *task.ActiveRunID != "run_recover_1" {
		t.Fatalf("activeRunID = %v, want run_recover_1", task.ActiveRunID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestResumeWithoutHandoffRejectsNonWatchdogBlockedTask(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	now := time.Now().UTC()
	mock.ExpectQuery("SELECT status FROM projects WHERE id = \\$1").
		WithArgs("prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	expectTaskGet(mock, "prj_1", "task_1", "blocked", "agt_1", nil, now)
	mock.ExpectQuery("SELECT COALESCE\\(error, ''\\) FROM tasks").
		WithArgs("prj_1", "task_1").
		WillReturnRows(sqlmock.NewRows([]string{"error"}).AddRow("manual_block"))

	_, err = svc.Resume(
		context.Background(),
		identity.Identity{SenderType: identity.SenderTypeAgent, Agent: identity.AgentIdentity{AgentID: "agt_1"}},
		"prj_1",
		"task_1",
		ResumeTaskInput{RunID: "run_recover_1"},
	)
	if !errors.Is(err, ErrTaskStatusInvalid) {
		t.Fatalf("Resume() error = %v, want ErrTaskStatusInvalid", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestResumeRejectsExistingRunOwnedByAnotherAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	now := time.Now().UTC()
	mock.ExpectQuery("SELECT status FROM projects WHERE id = \\$1").
		WithArgs("prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	expectTaskGet(mock, "prj_1", "task_1", "blocked", "agt_1", nil, now)
	mock.ExpectQuery("SELECT COALESCE\\(error, ''\\) FROM tasks").
		WithArgs("prj_1", "task_1").
		WillReturnRows(sqlmock.NewRows([]string{"error"}).AddRow(heartbeatTimeoutBlockReason))
	mock.ExpectQuery("SELECT COUNT\\(1\\)[\\s\\S]*FROM task_dependencies").
		WithArgs("task_1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT agent_id, project_id FROM agent_runs WHERE id = \\$1").
		WithArgs("run_other").
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "project_id"}).AddRow("agt_other", "prj_1"))

	_, err = svc.Resume(
		context.Background(),
		identity.Identity{SenderType: identity.SenderTypeAgent, Agent: identity.AgentIdentity{AgentID: "agt_1"}},
		"prj_1",
		"task_1",
		ResumeTaskInput{RunID: "run_other"},
	)
	if !errors.Is(err, ErrTaskActiveRunMismatch) {
		t.Fatalf("Resume() error = %v, want ErrTaskActiveRunMismatch", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestHeartbeatUpdatesTaskAndActiveRunHeartbeat(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	now := time.Now().UTC()
	actor := identity.Identity{SenderType: identity.SenderTypeAgent, Agent: identity.AgentIdentity{AgentID: "agt_1"}}

	mock.ExpectQuery("SELECT status FROM projects WHERE id = \\$1").
		WithArgs("prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	mock.ExpectQuery("SELECT context_state FROM agent_runs WHERE id = \\$1").
		WithArgs("run_1").
		WillReturnRows(sqlmock.NewRows([]string{"context_state"}).AddRow("normal"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks[\\s\\S]*SET last_heartbeat_at = NOW\\(\\)").
		WithArgs("prj_1", "task_1", "agt_1", "run_1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE agent_runs[\\s\\S]*SET last_heartbeat_at = NOW\\(\\)").
		WithArgs("run_1", "agt_1", "prj_1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectTaskGet(mock, "prj_1", "task_1", "running", "agt_1", stringPtr("run_1"), now)

	task, err := svc.Heartbeat(context.Background(), actor, "prj_1", "task_1", HeartbeatTaskInput{RunID: "run_1"})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if task.Status != StatusRunning {
		t.Fatalf("status = %s, want %s", task.Status, StatusRunning)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestBlockTimedOutRunningTasksKeepsTaskWhenActiveRunHeartbeatIsFresh(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks t[\\s\\S]*LEFT JOIN agent_runs ar ON ar.id = t.active_run_id").
		WithArgs(sqlmock.AnyArg(), 200).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "session_id", "active_run_id"}))

	blocked, err := svc.BlockTimedOutRunningTasks(context.Background(), 90*time.Second, 200)
	if err != nil {
		t.Fatalf("BlockTimedOutRunningTasks() error = %v", err)
	}
	if blocked != 0 {
		t.Fatalf("blocked = %d, want 0", blocked)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestBlockTimedOutRunningTasksBlocksStaleRunningTask(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks t[\\s\\S]*LEFT JOIN agent_runs ar ON ar.id = t.active_run_id").
		WithArgs(sqlmock.AnyArg(), 200).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "session_id", "active_run_id"}).
			AddRow("task_1", "prj_1", "sess_1", "run_1"))
	mock.ExpectExec("UPDATE tasks[\\s\\S]*SET status = 'blocked'").
		WithArgs("prj_1", "task_1", heartbeatTimeoutBlockReason).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE agent_runs[\\s\\S]*end_reason = 'heartbeat_timeout'").
		WithArgs("run_1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO task_events").
		WillReturnResult(sqlmock.NewResult(0, 1))

	blocked, err := svc.BlockTimedOutRunningTasks(context.Background(), 90*time.Second, 200)
	if err != nil {
		t.Fatalf("BlockTimedOutRunningTasks() error = %v", err)
	}
	if blocked != 1 {
		t.Fatalf("blocked = %d, want 1", blocked)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestValidateCreateInputIgnoresDelegateType(t *testing.T) {
	details := validateCreateInput("prj_1", CreateTaskInput{
		Title:       "ok title",
		Description: "ok desc",
	})
	if got := details["delegateToAgentId"]; got != "" {
		t.Fatalf("delegateToAgentId validation should be ignored when only type provided")
	}
}

func expectTaskGet(mock sqlmock.Sqlmock, projectID string, taskID string, status string, assigneeAgentID string, activeRunID *string, now time.Time) {
	activeRunValue := any(nil)
	if activeRunID != nil {
		activeRunValue = *activeRunID
	}
	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks[\\s\\S]*WHERE project_id = \\$1 AND id = \\$2").
		WithArgs(projectID, taskID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "title", "description", "goal", "input_context", "constraints_text", "status", "parent_task_id",
			"assignee_agent_id", "assignee_agent_type", "delegated_by_type", "delegated_by_operator_label",
			"delegated_by_agent_id", "delegated_at", "active_run_id", "last_heartbeat_at",
			"required_model", "output_contract", "priority", "created_at", "updated_at",
		}).AddRow(
			taskID, projectID, "title", "desc", nil, nil, nil, status, nil,
			assigneeAgentID, "codex", "operator", "local-operator",
			nil, now, activeRunValue, now,
			nil, nil, 1, now, now,
		))
	mock.ExpectQuery("SELECT depends_on_task_id FROM task_dependencies").
		WithArgs(taskID).
		WillReturnRows(sqlmock.NewRows([]string{"depends_on_task_id"}))
	mock.ExpectQuery("SELECT skill_name FROM task_required_skills").
		WithArgs(taskID).
		WillReturnRows(sqlmock.NewRows([]string{"skill_name"}))
}

func stringPtr(value string) *string {
	return &value
}

func TestCreateRejectsArchivedProject(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectQuery("SELECT status FROM projects WHERE id = \\$1").
		WithArgs("prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("archived"))

	_, err = svc.Create(context.Background(), identity.Operator("local-operator"), "prj_1", CreateTaskInput{
		Title:       "ok title",
		Description: "ok description",
	})
	if !errors.Is(err, ErrProjectArchived) {
		t.Fatalf("Create() error = %v, want %v", err, ErrProjectArchived)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestReviewAllowsOperatorWithoutTaskReviewScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	now := time.Now().UTC()
	reviewRows := sqlmock.NewRows([]string{
		"id", "project_id", "title", "description", "goal", "input_context", "constraints_text", "status", "parent_task_id",
		"assignee_agent_id", "assignee_agent_type", "delegated_by_type", "delegated_by_operator_label",
		"delegated_by_agent_id", "delegated_at", "active_run_id", "last_heartbeat_at",
		"required_model", "output_contract", "priority", "created_at", "updated_at",
	}).AddRow(
		"task_review_1", "prj_1", "Review submitted task", "review", nil, nil, nil, "delegated", "task_parent_1",
		"agt_reviewer", "claude-code", "system", nil,
		nil, now, nil, nil,
		nil, nil, 80, now, now,
	)
	parentRows := sqlmock.NewRows([]string{
		"id", "project_id", "title", "description", "goal", "input_context", "constraints_text", "status", "parent_task_id",
		"assignee_agent_id", "assignee_agent_type", "delegated_by_type", "delegated_by_operator_label",
		"delegated_by_agent_id", "delegated_at", "active_run_id", "last_heartbeat_at",
		"required_model", "output_contract", "priority", "created_at", "updated_at",
	}).AddRow(
		"task_parent_1", "prj_1", "Parent task", "impl", nil, nil, nil, "submitted", nil,
		"agt_worker", "codex", "agent", nil,
		"agt_worker", now, "run_1", now,
		nil, nil, 70, now, now,
	)
	doneParentRows := sqlmock.NewRows([]string{
		"id", "project_id", "title", "description", "goal", "input_context", "constraints_text", "status", "parent_task_id",
		"assignee_agent_id", "assignee_agent_type", "delegated_by_type", "delegated_by_operator_label",
		"delegated_by_agent_id", "delegated_at", "active_run_id", "last_heartbeat_at",
		"required_model", "output_contract", "priority", "created_at", "updated_at",
	}).AddRow(
		"task_parent_1", "prj_1", "Parent task", "impl", nil, nil, nil, "done", nil,
		"agt_worker", "codex", "agent", nil,
		"agt_worker", now, nil, now,
		nil, nil, 70, now, now,
	)

	mock.ExpectQuery("SELECT status FROM projects WHERE id = \\$1").
		WithArgs("prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks[\\s\\S]*WHERE project_id = \\$1 AND id = \\$2").
		WithArgs("prj_1", "task_review_1").
		WillReturnRows(reviewRows)
	mock.ExpectQuery("SELECT depends_on_task_id FROM task_dependencies").WithArgs("task_review_1").
		WillReturnRows(sqlmock.NewRows([]string{"depends_on_task_id"}))
	mock.ExpectQuery("SELECT skill_name FROM task_required_skills").WithArgs("task_review_1").
		WillReturnRows(sqlmock.NewRows([]string{"skill_name"}))

	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks[\\s\\S]*WHERE project_id = \\$1 AND id = \\$2").
		WithArgs("prj_1", "task_parent_1").
		WillReturnRows(parentRows)
	mock.ExpectQuery("SELECT depends_on_task_id FROM task_dependencies").WithArgs("task_parent_1").
		WillReturnRows(sqlmock.NewRows([]string{"depends_on_task_id"}))
	mock.ExpectQuery("SELECT skill_name FROM task_required_skills").WithArgs("task_parent_1").
		WillReturnRows(sqlmock.NewRows([]string{"skill_name"}))

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE tasks SET status = 'done'").
		WithArgs("prj_1", "task_review_1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE tasks[\\s\\S]*SET status = \\$3").
		WithArgs("prj_1", "task_parent_1", "done", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO task_events").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery("SELECT[\\s\\S]*FROM tasks[\\s\\S]*WHERE project_id = \\$1 AND id = \\$2").
		WithArgs("prj_1", "task_parent_1").
		WillReturnRows(doneParentRows)
	mock.ExpectQuery("SELECT depends_on_task_id FROM task_dependencies").WithArgs("task_parent_1").
		WillReturnRows(sqlmock.NewRows([]string{"depends_on_task_id"}))
	mock.ExpectQuery("SELECT skill_name FROM task_required_skills").WithArgs("task_parent_1").
		WillReturnRows(sqlmock.NewRows([]string{"skill_name"}))

	item, err := svc.Review(
		context.Background(),
		identity.Operator("local-operator"),
		"prj_1",
		"task_review_1",
		ReviewTaskInput{Approve: true, Reason: "approved by operator"},
	)
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if got, want := item.Status, StatusDone; got != want {
		t.Fatalf("Review() status = %q, want %q", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestReviewRejectsAgentWithoutTaskReviewScope(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, ConsoleOperatorLabel: "local-operator"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = svc.Review(
		context.Background(),
		identity.Identity{
			SenderType: identity.SenderTypeAgent,
			Agent: identity.AgentIdentity{
				AgentID: "agt_1",
				Scopes:  []string{"task:read:own"},
			},
		},
		"prj_1",
		"task_review_1",
		ReviewTaskInput{Approve: true, Reason: "ok"},
	)
	if !errors.Is(err, ErrReviewScopeRequired) {
		t.Fatalf("Review() error = %v, want %v", err, ErrReviewScopeRequired)
	}
}
