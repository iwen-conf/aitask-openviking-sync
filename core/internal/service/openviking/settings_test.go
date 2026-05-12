package openviking

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEncryptDecryptAPIKeyRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	ciphertext, err := encryptAPIKey(key, "secret-key")
	if err != nil {
		t.Fatalf("encryptAPIKey() error = %v", err)
	}
	if string(ciphertext) == "secret-key" {
		t.Fatalf("ciphertext contains plaintext")
	}
	plaintext, err := decryptAPIKey(key, ciphertext)
	if err != nil {
		t.Fatalf("decryptAPIKey() error = %v", err)
	}
	if got, want := plaintext, "secret-key"; got != want {
		t.Fatalf("plaintext = %q, want %q", got, want)
	}
}

func TestSettingsStoreGetSystemDecryptsAPIKey(t *testing.T) {
	db, mock, cleanup := newSettingsMockDB(t)
	defer cleanup()
	key := []byte("0123456789abcdef0123456789abcdef")
	ciphertext, err := encryptAPIKey(key, "secret-key")
	if err != nil {
		t.Fatalf("encryptAPIKey() error = %v", err)
	}
	store, err := NewSettingsStore(db, key)
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}

	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"server_url",
		"enable_memory_write", "enable_auto_sync",
		"api_key_ciphertext", "last_sync_at", "last_error",
	}).AddRow("http://ov:9090", true, false, ciphertext, now, "last failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT server_url,
		       enable_memory_write, enable_auto_sync,
		       api_key_ciphertext, last_sync_at, last_error
		FROM system_openviking_settings
		WHERE id = TRUE
	`)).WillReturnRows(rows)

	settings, err := store.GetSystem(context.Background())
	if err != nil {
		t.Fatalf("GetSystem() error = %v", err)
	}
	if got, want := settings.ApiKey, "secret-key"; got != want {
		t.Fatalf("ApiKey = %q, want %q", got, want)
	}
	if !settings.ApiKeySet {
		t.Fatalf("ApiKeySet = false, want true")
	}
	if settings.LastSyncAt == nil || !settings.LastSyncAt.Equal(now) {
		t.Fatalf("LastSyncAt = %v, want %v", settings.LastSyncAt, now)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSettingsStoreUpsertSystemKeepsClearsAndOverwritesAPIKey(t *testing.T) {
	db, mock, cleanup := newSettingsMockDB(t)
	defer cleanup()
	key := []byte("0123456789abcdef0123456789abcdef")
	store, err := NewSettingsStore(db, key)
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}

	empty := ""
	mock.ExpectExec("INSERT INTO system_openviking_settings").
		WithArgs("http://ov:9090", true, true, "ops").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSystemSettingsGet(mock, nil)
	settings, err := store.UpsertSystem(context.Background(), UpsertSettingsInput{
		ServerURL:         "http://ov:9090",
		EnableMemoryWrite: true,
		EnableAutoSync:    true,
		ApiKey:            &empty,
	}, "ops")
	if err != nil {
		t.Fatalf("UpsertSystem(empty) error = %v", err)
	}
	if settings.ApiKeySet {
		t.Fatalf("ApiKeySet after empty update = true, want false")
	}

	clear := "null"
	mock.ExpectExec("INSERT INTO system_openviking_settings").
		WithArgs("http://ov:9090", true, true, nil, "ops").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSystemSettingsGet(mock, nil)
	if _, err := store.UpsertSystem(context.Background(), UpsertSettingsInput{
		ServerURL:         "http://ov:9090",
		EnableMemoryWrite: true,
		EnableAutoSync:    true,
		ApiKey:            &clear,
	}, "ops"); err != nil {
		t.Fatalf("UpsertSystem(clear) error = %v", err)
	}

	overwrite := "new-secret"
	mock.ExpectExec("INSERT INTO system_openviking_settings").
		WithArgs("http://ov:9090", true, true, sqlmock.AnyArg(), "ops").
		WillReturnResult(sqlmock.NewResult(0, 1))
	ciphertext, err := encryptAPIKey(key, overwrite)
	if err != nil {
		t.Fatalf("encryptAPIKey() error = %v", err)
	}
	expectSystemSettingsGet(mock, ciphertext)
	settings, err = store.UpsertSystem(context.Background(), UpsertSettingsInput{
		ServerURL:         "http://ov:9090",
		EnableMemoryWrite: true,
		EnableAutoSync:    true,
		ApiKey:            &overwrite,
	}, "ops")
	if err != nil {
		t.Fatalf("UpsertSystem(overwrite) error = %v", err)
	}
	if got, want := settings.ApiKey, overwrite; got != want {
		t.Fatalf("ApiKey = %q, want %q", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestProjectAwareWriterDisabledMemoryWriteSkipsSync(t *testing.T) {
	db, mock, cleanup := newSettingsMockDB(t)
	defer cleanup()
	store, err := NewSettingsStore(db, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}
	rows := sqlmock.NewRows([]string{
		"server_url",
		"enable_memory_write", "enable_auto_sync",
		"api_key_ciphertext", "last_sync_at", "last_error",
	}).AddRow("http://system-ov:9090", false, true, nil, nil, nil)
	mock.ExpectQuery("SELECT server_url").WillReturnRows(rows)
	mock.ExpectQuery("SELECT openviking_namespace FROM projects").
		WithArgs("prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"openviking_namespace"}).AddRow("aitask"))
	writer := NewProjectAwareWriter(NewProjectClientFactory(store, nil, db, "aitask"), nil)

	result, err := writer.Write(context.Background(), "prj_1", WriteInput{Target: "summary", Title: "t", Content: "c"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if result.Synced {
		t.Fatalf("Synced = true, want false")
	}
}

func TestProjectAwareWriterAutoSyncDisabledSkipsOnlyAutomaticWrites(t *testing.T) {
	db, mock, cleanup := newSettingsMockDB(t)
	defer cleanup()
	store, err := NewSettingsStore(db, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}
	rows := sqlmock.NewRows([]string{
		"server_url",
		"enable_memory_write", "enable_auto_sync",
		"api_key_ciphertext", "last_sync_at", "last_error",
	}).AddRow("http://system-ov:9090", true, false, nil, nil, nil)
	mock.ExpectQuery("SELECT server_url").WillReturnRows(rows)
	mock.ExpectQuery("SELECT openviking_namespace FROM projects").
		WithArgs("prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"openviking_namespace"}).AddRow("aitask"))
	writer := NewProjectAwareWriter(NewProjectClientFactory(store, nil, db, "aitask"), nil)

	result, err := writer.Write(context.Background(), "prj_1", WriteInput{Target: "summary", Title: "t", Content: "c", AutoSync: true})
	if err != nil {
		t.Fatalf("Write(auto) error = %v", err)
	}
	if result.Synced {
		t.Fatalf("Synced = true, want false")
	}
}

func TestProjectAwareWriterNotConfiguredReturnsError(t *testing.T) {
	db, mock, cleanup := newSettingsMockDB(t)
	defer cleanup()
	store, err := NewSettingsStore(db, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}
	mock.ExpectQuery("SELECT server_url").WillReturnError(sql.ErrNoRows)
	writer := NewProjectAwareWriter(NewProjectClientFactory(store, nil, db, "aitask"), nil)

	_, err = writer.Write(context.Background(), "prj_1", WriteInput{Target: "summary", Title: "t", Content: "c"})
	if err == nil {
		t.Fatalf("Write() expected error when settings not configured")
	}
	var ovErr *Error
	if !errors.As(err, &ovErr) || ovErr.Kind != ErrorKindUnavailable {
		t.Fatalf("Write() error = %v, want ErrorKindUnavailable", err)
	}
}

func TestProjectAwareWriterEmptyServerURLReturnsError(t *testing.T) {
	db, mock, cleanup := newSettingsMockDB(t)
	defer cleanup()
	store, err := NewSettingsStore(db, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}
	rows := sqlmock.NewRows([]string{
		"server_url",
		"enable_memory_write", "enable_auto_sync",
		"api_key_ciphertext", "last_sync_at", "last_error",
	}).AddRow("", true, true, nil, nil, nil)
	mock.ExpectQuery("SELECT server_url").WillReturnRows(rows)
	writer := NewProjectAwareWriter(NewProjectClientFactory(store, nil, db, "aitask"), nil)

	_, err = writer.Write(context.Background(), "prj_1", WriteInput{Target: "summary", Title: "t", Content: "c"})
	if err == nil {
		t.Fatalf("Write() expected error when ServerURL empty")
	}
	var ovErr *Error
	if !errors.As(err, &ovErr) || ovErr.Kind != ErrorKindUnavailable {
		t.Fatalf("Write() error = %v, want ErrorKindUnavailable", err)
	}
}

func TestProjectAwareWriterRegisterGitResourceUsesProjectSettings(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{"uri":"viking://resources/aitask"}}`))
	}))
	defer ts.Close()

	db, mock, cleanup := newSettingsMockDB(t)
	defer cleanup()
	store, err := NewSettingsStore(db, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}
	rows := sqlmock.NewRows([]string{
		"server_url",
		"enable_memory_write", "enable_auto_sync",
		"api_key_ciphertext", "last_sync_at", "last_error",
	}).AddRow(ts.URL, true, true, nil, nil, nil)
	mock.ExpectQuery("SELECT server_url").WillReturnRows(rows)
	mock.ExpectQuery("SELECT openviking_namespace FROM projects").
		WithArgs("prj_1").
		WillReturnRows(sqlmock.NewRows([]string{"openviking_namespace"}).AddRow("aitask"))
	writer := NewProjectAwareWriter(NewProjectClientFactory(store, ts.Client(), db, "aitask"), nil)

	result, err := writer.RegisterGitResource(context.Background(), "prj_1", GitResourceInput{
		RepositoryURL: "git@example.com:org/aitask.git",
		TargetURI:     "viking://resources/aitask",
	})
	if err != nil {
		t.Fatalf("RegisterGitResource() error = %v", err)
	}
	if gotPath != "/api/v1/resources" {
		t.Fatalf("path = %q, want /api/v1/resources", gotPath)
	}
	if result.URI != "viking://resources/aitask" || !result.Synced {
		t.Fatalf("unexpected result: %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestClientSendsAPIKeyBearerToken(t *testing.T) {
	var authHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask", APIKey: "secret"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.List(context.Background(), "prj_1"); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := authHeader, "Bearer secret"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
}

func TestSettingsStoreUpsertSystemNormalizesConsoleURL(t *testing.T) {
	db, mock, cleanup := newSettingsMockDB(t)
	defer cleanup()
	key := []byte("0123456789abcdef0123456789abcdef")
	store, err := NewSettingsStore(db, key)
	if err != nil {
		t.Fatalf("NewSettingsStore() error = %v", err)
	}

	mock.ExpectExec("INSERT INTO system_openviking_settings").
		WithArgs("https://openviking.example.com/ov-api", true, true, "ops").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSystemSettingsGet(mock, nil)

	_, err = store.UpsertSystem(context.Background(), UpsertSettingsInput{
		ServerURL:         "https://openviking.example.com/console/",
		EnableMemoryWrite: true,
		EnableAutoSync:    true,
	}, "ops")
	if err != nil {
		t.Fatalf("UpsertSystem() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func newSettingsMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	return db, mock, func() { _ = db.Close() }
}

func expectSystemSettingsGet(mock sqlmock.Sqlmock, ciphertext []byte) {
	rows := sqlmock.NewRows([]string{
		"server_url",
		"enable_memory_write", "enable_auto_sync",
		"api_key_ciphertext", "last_sync_at", "last_error",
	}).AddRow("http://ov:9090", true, true, ciphertext, nil, nil)
	mock.ExpectQuery("SELECT server_url").WillReturnRows(rows)
}
