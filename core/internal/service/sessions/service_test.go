package sessions

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetActiveSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(db)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	updatedAt := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "project_id", "status", "current_phase", "context_summary", "updated_at"}).
		AddRow("sess_1", "prj_1", "active", "implementation", "summary", updatedAt)
	mock.ExpectQuery("SELECT[\\s\\S]*FROM projects p").WithArgs("prj_1").WillReturnRows(rows)

	output, err := svc.GetActive(context.Background(), "prj_1")
	if err != nil {
		t.Fatalf("GetActive() error = %v", err)
	}
	if got, want := output.SessionID, "sess_1"; got != want {
		t.Fatalf("SessionID = %q, want %q", got, want)
	}
	if got, want := output.CurrentPhase, "implementation"; got != want {
		t.Fatalf("CurrentPhase = %q, want %q", got, want)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestGetActiveProjectNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(db)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectQuery("SELECT[\\s\\S]*FROM projects p").WithArgs("prj_missing").WillReturnError(sql.ErrNoRows)
	existsRows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM projects`).WithArgs("prj_missing").WillReturnRows(existsRows)

	_, err = svc.GetActive(context.Background(), "prj_missing")
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("GetActive() error = %v, want %v", err, ErrProjectNotFound)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestSwitchPhaseSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(db)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectQuery("SELECT active_session_id FROM projects").WithArgs("prj_1").WillReturnRows(
		sqlmock.NewRows([]string{"active_session_id"}).AddRow("sess_1"),
	)
	mock.ExpectExec("UPDATE project_sessions").WithArgs("sess_1", "review").WillReturnResult(sqlmock.NewResult(0, 1))
	rows := sqlmock.NewRows([]string{"id", "project_id", "status", "current_phase", "context_summary", "updated_at"}).
		AddRow("sess_1", "prj_1", "active", "review", "summary", time.Now().UTC())
	mock.ExpectQuery("SELECT[\\s\\S]*FROM projects p").WithArgs("prj_1").WillReturnRows(rows)

	output, err := svc.SwitchPhase(context.Background(), "prj_1", "review")
	if err != nil {
		t.Fatalf("SwitchPhase() error = %v", err)
	}
	if got, want := output.CurrentPhase, "review"; got != want {
		t.Fatalf("CurrentPhase = %q, want %q", got, want)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}
