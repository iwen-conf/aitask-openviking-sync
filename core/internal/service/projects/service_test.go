package projects

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateProjectSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	openVikingHookCalled := false
	roomHookCalled := false
	agentHookCalled := false

	svc, err := New(Options{
		DB:                   db,
		ConsoleOperatorLabel: "local-operator",
		OpenVikingNamespace:  "aitask",
		Hooks: CreateProjectHooks{
			InitializeOpenViking: func(_ context.Context, _ *sql.Tx, payload HookPayload) error {
				openVikingHookCalled = true
				if !strings.HasPrefix(payload.ProjectID, "prj_") {
					t.Fatalf("payload.ProjectID = %q, want prefix prj_", payload.ProjectID)
				}
				return nil
			},
			InitializeProjectRoom: func(_ context.Context, _ *sql.Tx, payload HookPayload) (string, error) {
				roomHookCalled = true
				if !strings.HasPrefix(payload.RoomID, "room_") {
					t.Fatalf("payload.RoomID = %q, want prefix room_", payload.RoomID)
				}
				return "room_hooked", nil
			},
			BindDefaultAgents: func(_ context.Context, _ *sql.Tx, payload HookPayload) error {
				agentHookCalled = true
				if !strings.HasPrefix(payload.RootTaskID, "task_") {
					t.Fatalf("payload.RootTaskID = %q, want prefix task_", payload.RootTaskID)
				}
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO projects").WithArgs(
		sqlmock.AnyArg(),
		"AgentTaskSystem",
		"Persistent task orchestration",
		"Build a persistent AI Agent project orchestration system",
		"active",
		"local-operator",
		sqlmock.AnyArg(),
		"aitask",
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec("INSERT INTO project_sessions").WithArgs(
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		"active",
		"project_bootstrap",
	).WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec("INSERT INTO tasks").WithArgs(
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		"Project bootstrap",
		sqlmock.AnyArg(),
		"planned",
		"operator",
		"local-operator",
	).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	output, err := svc.Create(context.Background(), CreateProjectInput{
		Name:        "AgentTaskSystem",
		Goal:        "Build a persistent AI Agent project orchestration system",
		Description: "Persistent task orchestration",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !openVikingHookCalled || !roomHookCalled || !agentHookCalled {
		t.Fatalf("hooks called = openviking:%t room:%t agents:%t, want all true", openVikingHookCalled, roomHookCalled, agentHookCalled)
	}

	if !strings.HasPrefix(output.ProjectID, "prj_") {
		t.Fatalf("output.ProjectID = %q, want prefix prj_", output.ProjectID)
	}
	if !strings.HasPrefix(output.ActiveSessionID, "sess_") {
		t.Fatalf("output.ActiveSessionID = %q, want prefix sess_", output.ActiveSessionID)
	}
	if got, want := output.RoomID, "room_hooked"; got != want {
		t.Fatalf("output.RoomID = %q, want %q", got, want)
	}
	if got, want := output.Status, "active"; got != want {
		t.Fatalf("output.Status = %q, want %q", got, want)
	}
	if got, want := output.InitCommand, "aitask init --project "+output.ProjectID; got != want {
		t.Fatalf("output.InitCommand = %q, want %q", got, want)
	}
	if got, want := output.OpenVikingRoot, "viking://aitask/projects/"+output.ProjectID; got != want {
		t.Fatalf("output.OpenVikingRoot = %q, want %q", got, want)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestCreateProjectRollsBackOnHookError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{
		DB:                   db,
		ConsoleOperatorLabel: "local-operator",
		OpenVikingNamespace:  "aitask",
		Hooks: CreateProjectHooks{
			InitializeOpenViking: func(context.Context, *sql.Tx, HookPayload) error {
				return errors.New("openviking unavailable")
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO projects").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO project_sessions").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO tasks").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectRollback()

	_, err = svc.Create(context.Background(), CreateProjectInput{
		Name: "AgentTaskSystem",
		Goal: "Build a persistent AI Agent project orchestration system",
	})
	if !errors.Is(err, ErrCreateProjectFailed) {
		t.Fatalf("Create() error = %v, want wrapped %v", err, ErrCreateProjectFailed)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestCreateProjectRejectsInvalidInput(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{
		DB:                   db,
		ConsoleOperatorLabel: "local-operator",
		OpenVikingNamespace:  "aitask",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = svc.Create(context.Background(), CreateProjectInput{
		Name: "A",
		Goal: "x",
	})
	if err == nil {
		t.Fatal("Create() error = nil, want invalid input error")
	}

	var invalid *InvalidInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("Create() error = %v, want InvalidInputError", err)
	}
	if got := invalid.Details()["name"]; got == "" {
		t.Fatalf("invalid details missing name field")
	}
	if got := invalid.Details()["goal"]; got == "" {
		t.Fatalf("invalid details missing goal field")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected DB interaction: %v", err)
	}
}

func TestArchiveProjectSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{
		DB:                   db,
		ConsoleOperatorLabel: "local-operator",
		OpenVikingNamespace:  "aitask",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	updatedAt := time.Now().UTC()
	mock.ExpectQuery("UPDATE projects").
		WithArgs("prj_1", "archived").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "updated_at"}).AddRow("prj_1", "archived", updatedAt))

	output, err := svc.Archive(context.Background(), "prj_1", ArchiveProjectInput{
		Confirm: true,
		Reason:  "release finished",
	})
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if got, want := output.ProjectID, "prj_1"; got != want {
		t.Fatalf("ProjectID = %q, want %q", got, want)
	}
	if got, want := output.Status, "archived"; got != want {
		t.Fatalf("Status = %q, want %q", got, want)
	}
	if !output.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("UpdatedAt = %v, want %v", output.UpdatedAt, updatedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestAssertWritableRejectsArchivedProject(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{
		DB:                   db,
		ConsoleOperatorLabel: "local-operator",
		OpenVikingNamespace:  "aitask",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectQuery("SELECT status FROM projects WHERE id = \\$1").
		WithArgs("prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("archived"))

	err = svc.AssertWritable(context.Background(), "prj_1")
	if !errors.Is(err, ErrProjectArchived) {
		t.Fatalf("AssertWritable() error = %v, want %v", err, ErrProjectArchived)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
