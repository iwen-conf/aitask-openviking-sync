package agents

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateRejectsInvalidAgentType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	svc, err := New(Options{DB: db, TokenSecret: "secret"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = svc.Create(context.Background(), CreateAgentInput{
		Name:      "agent-1",
		AgentType: "unknown",
		Role:      "worker",
	})
	if err == nil {
		t.Fatal("Create() error = nil, want error")
	}
	var invalid *InvalidInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("Create() error = %T, want *InvalidInputError", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestIssueTokenSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	svc, err := New(Options{DB: db, TokenSecret: "secret", Now: func() time.Time { return now }, Random: strings.NewReader(strings.Repeat("a", 24))})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectQuery("SELECT agent_type FROM agents").WithArgs("agt_1").WillReturnRows(
		sqlmock.NewRows([]string{"agent_type"}).AddRow("codex"),
	)
	mock.ExpectExec("INSERT INTO agent_tokens").WithArgs(
		sqlmock.AnyArg(),
		"agt_1",
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(1, 1))

	out, err := svc.IssueToken(context.Background(), "agt_1", IssueTokenInput{Scopes: []string{"task:read:delegated"}})
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}
	if !strings.HasPrefix(out.AgentToken, tokenPrefix) {
		t.Fatalf("AgentToken = %q, want prefix %q", out.AgentToken, tokenPrefix)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestVerifyTokenSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	svc, err := New(Options{DB: db, TokenSecret: "secret", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	token := tokenPrefix + "tok_1.xxxxx"
	hash := svc.tokenHash(token)
	expiresAt := now.Add(24 * time.Hour)

	mock.ExpectQuery("SELECT t.id, t.agent_id").WithArgs(hash).WillReturnRows(
		sqlmock.NewRows([]string{"id", "agent_id", "scopes", "expires_at", "revoked_at", "agent_type", "role", "status"}).
			AddRow("tok_1", "agt_1", `["task:read:delegated","task:start:delegated"]`, expiresAt, nil, "codex", "worker", "active"),
	)
	mock.ExpectQuery("SELECT project_id FROM agent_project_bindings").WithArgs("agt_1").WillReturnRows(
		sqlmock.NewRows([]string{"project_id"}).AddRow("prj_1").AddRow("prj_2"),
	)

	idn, err := svc.VerifyToken(context.Background(), token)
	if err != nil {
		t.Fatalf("VerifyToken() error = %v", err)
	}
	if got, want := idn.AgentID, "agt_1"; got != want {
		t.Fatalf("AgentID = %q, want %q", got, want)
	}
	if !idn.CanAccessProject("prj_2") {
		t.Fatalf("CanAccessProject(prj_2) = false, want true")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}

func TestVerifyTokenExpired(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	svc, err := New(Options{DB: db, TokenSecret: "secret", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	token := tokenPrefix + "tok_1.xxxxx"
	hash := svc.tokenHash(token)
	expiresAt := now.Add(-1 * time.Hour)

	mock.ExpectQuery("SELECT t.id, t.agent_id").WithArgs(hash).WillReturnRows(
		sqlmock.NewRows([]string{"id", "agent_id", "scopes", "expires_at", "revoked_at", "agent_type", "role", "status"}).
			AddRow("tok_1", "agt_1", `["task:read:delegated"]`, expiresAt, nil, "codex", "worker", "active"),
	)

	_, err = svc.VerifyToken(context.Background(), token)
	if !errors.Is(err, ErrAgentTokenExpired) {
		t.Fatalf("VerifyToken() error = %v, want %v", err, ErrAgentTokenExpired)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}
