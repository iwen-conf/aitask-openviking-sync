package room

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
)

func TestSendMessageRejectsArchivedProject(t *testing.T) {
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

	_, err = svc.SendMessage(context.Background(), identity.Operator("local-operator"), "prj_1", SendMessageInput{
		MessageType: "text",
		Content:     "hello",
	})
	if !errors.Is(err, ErrProjectArchived) {
		t.Fatalf("SendMessage() error = %v, want %v", err, ErrProjectArchived)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet() error = %v", err)
	}
}
