package openviking

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

var ErrSettingsNotFound = errors.New("openviking settings not found")

type Settings struct {
	ServerURL         string
	EnableMemoryWrite bool
	EnableAutoSync    bool
	ApiKey            string
	LastSyncAt        *time.Time
	LastError         string
	ApiKeySet         bool
	loaded            bool
}

type UpsertSettingsInput struct {
	ServerURL         string
	EnableMemoryWrite bool
	EnableAutoSync    bool
	ApiKey            *string
}

type SettingsStore struct {
	db  *sql.DB
	key []byte
}

func NewSettingsStore(db *sql.DB, key []byte) (*SettingsStore, error) {
	if db == nil {
		return nil, errors.New("openviking settings store requires database handle")
	}
	if len(key) != 32 {
		return nil, errors.New("openviking settings key must be 32 bytes")
	}
	keyCopy := append([]byte(nil), key...)
	return &SettingsStore{db: db, key: keyCopy}, nil
}

func (s *SettingsStore) GetSystem(ctx context.Context) (Settings, error) {
	var out Settings
	out.loaded = true
	var ciphertext []byte
	var lastSyncAt sql.NullTime
	var lastError sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT server_url,
		       enable_memory_write, enable_auto_sync,
		       api_key_ciphertext, last_sync_at, last_error
		FROM system_openviking_settings
		WHERE id = TRUE
	`).Scan(
		&out.ServerURL,
		&out.EnableMemoryWrite,
		&out.EnableAutoSync,
		&ciphertext,
		&lastSyncAt,
		&lastError,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, ErrSettingsNotFound
	}
	if err != nil {
		return Settings{}, fmt.Errorf("get system openviking settings failed: %w", err)
	}
	if lastSyncAt.Valid {
		at := lastSyncAt.Time.UTC()
		out.LastSyncAt = &at
	}
	if lastError.Valid {
		out.LastError = lastError.String
	}
	if len(ciphertext) > 0 {
		apiKey, err := decryptAPIKey(s.key, ciphertext)
		if err != nil {
			return Settings{}, fmt.Errorf("decrypt system openviking api key failed: %w", err)
		}
		out.ApiKey = apiKey
		out.ApiKeySet = strings.TrimSpace(apiKey) != ""
	}
	return out, nil
}

func (s *SettingsStore) UpsertSystem(ctx context.Context, input UpsertSettingsInput, actorID string) (Settings, error) {
	input.ServerURL = strings.TrimSpace(input.ServerURL)
	actorID = strings.TrimSpace(actorID)
	if err := validateSettingsInput(input); err != nil {
		return Settings{}, err
	}
	normalizedServerURL, err := NormalizeServerURL(input.ServerURL)
	if err != nil {
		return Settings{}, &Error{Op: "settings_validate", Kind: ErrorKindBadRequest, Err: err}
	}
	input.ServerURL = normalizedServerURL

	var ciphertext any
	var updateKey bool
	if input.ApiKey != nil {
		raw := strings.TrimSpace(*input.ApiKey)
		if raw == "" {
			updateKey = false
		} else if strings.EqualFold(raw, "null") {
			updateKey = true
			ciphertext = nil
		} else {
			updateKey = true
			encrypted, err := encryptAPIKey(s.key, raw)
			if err != nil {
				return Settings{}, fmt.Errorf("encrypt system openviking api key failed: %w", err)
			}
			ciphertext = encrypted
		}
	}

	if updateKey {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO system_openviking_settings (
				id, server_url,
				enable_memory_write, enable_auto_sync,
				api_key_ciphertext, updated_at, updated_by
			) VALUES (TRUE,$1,$2,$3,$4,NOW(),$5)
			ON CONFLICT (id)
			DO UPDATE SET
				server_url = EXCLUDED.server_url,
				enable_memory_write = EXCLUDED.enable_memory_write,
				enable_auto_sync = EXCLUDED.enable_auto_sync,
				api_key_ciphertext = EXCLUDED.api_key_ciphertext,
				updated_at = NOW(),
				updated_by = EXCLUDED.updated_by
		`, input.ServerURL, input.EnableMemoryWrite, input.EnableAutoSync, ciphertext, actorID)
		if err != nil {
			return Settings{}, fmt.Errorf("upsert system openviking settings failed: %w", err)
		}
	} else {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO system_openviking_settings (
				id, server_url,
				enable_memory_write, enable_auto_sync,
				updated_at, updated_by
			) VALUES (TRUE,$1,$2,$3,NOW(),$4)
			ON CONFLICT (id)
			DO UPDATE SET
				server_url = EXCLUDED.server_url,
				enable_memory_write = EXCLUDED.enable_memory_write,
				enable_auto_sync = EXCLUDED.enable_auto_sync,
				updated_at = NOW(),
				updated_by = EXCLUDED.updated_by
		`, input.ServerURL, input.EnableMemoryWrite, input.EnableAutoSync, actorID)
		if err != nil {
			return Settings{}, fmt.Errorf("upsert system openviking settings failed: %w", err)
		}
	}

	return s.GetSystem(ctx)
}

func validateSettingsInput(input UpsertSettingsInput) error {
	if _, err := ServerURLCandidates(input.ServerURL); err != nil {
		return &Error{Op: "settings_validate", Kind: ErrorKindBadRequest, Err: err}
	}
	return nil
}

func encryptAPIKey(key []byte, apiKey string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, []byte(apiKey), nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

func decryptAPIKey(key []byte, ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	sealed := ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

type ProjectClientFactory struct {
	store      *SettingsStore
	httpClient *http.Client
	db         *sql.DB
	fallbackNS string
}

func NewProjectClientFactory(store *SettingsStore, httpClient *http.Client, db *sql.DB, fallbackNamespace string) *ProjectClientFactory {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 8 * time.Second}
	}
	return &ProjectClientFactory{store: store, httpClient: httpClient, db: db, fallbackNS: strings.TrimSpace(fallbackNamespace)}
}

// projectNamespace looks up projects.openviking_namespace for the given
// project, falling back to the env-time namespace when the row is missing
// or the column is empty.
func (f *ProjectClientFactory) projectNamespace(ctx context.Context, projectID string) string {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || f == nil || f.db == nil {
		return f.fallbackNS
	}
	var ns string
	err := f.db.QueryRowContext(ctx, `SELECT openviking_namespace FROM projects WHERE id = $1`, projectID).Scan(&ns)
	if err != nil || strings.TrimSpace(ns) == "" {
		return f.fallbackNS
	}
	return strings.TrimSpace(ns)
}

// ProjectClient builds a per-project OpenViking client using system credentials
// (URL + API key) and the project-scoped namespace. Returns (nil, settings, nil)
// when system credentials are not configured so callers can fall back to global.
func (f *ProjectClientFactory) ProjectClient(ctx context.Context, projectID string) (*Client, Settings, error) {
	if f == nil || f.store == nil {
		return nil, Settings{}, nil
	}
	settings, err := f.store.GetSystem(ctx)
	if err != nil {
		if errors.Is(err, ErrSettingsNotFound) {
			return nil, Settings{}, nil
		}
		return nil, Settings{}, err
	}
	if strings.TrimSpace(settings.ServerURL) == "" {
		return nil, settings, nil
	}
	namespace := f.projectNamespace(ctx, projectID)
	if namespace == "" {
		return nil, settings, nil
	}
	client, err := New(Options{
		BaseURL:    settings.ServerURL,
		Namespace:  namespace,
		APIKey:     settings.ApiKey,
		HTTPClient: f.httpClient,
	})
	if err != nil {
		return nil, settings, err
	}
	return client, settings, nil
}

type ProjectAwareWriter struct {
	factory *ProjectClientFactory
	logger  *slog.Logger
}

type MemoryClient interface {
	List(ctx context.Context, projectID string) ([]ListItem, error)
	Search(ctx context.Context, projectID string, query string, budget int, refsOnly bool) ([]SearchHit, error)
	Read(ctx context.Context, projectID string, uri string) (ReadEntry, error)
	Write(ctx context.Context, projectID string, input WriteInput) (WriteResult, error)
}

type GitResourceRegistrar interface {
	RegisterGitResource(ctx context.Context, projectID string, input GitResourceInput) (GitResourceResult, error)
	ResourceSyncStatus(ctx context.Context, projectID string, query ResourceTaskQuery) (ResourceSyncStatus, error)
}

func NewProjectAwareWriter(factory *ProjectClientFactory, logger *slog.Logger) *ProjectAwareWriter {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProjectAwareWriter{factory: factory, logger: logger}
}

func errOpenVikingNotConfigured(op string) error {
	return &Error{
		Op:   op,
		Kind: ErrorKindUnavailable,
		Err:  errors.New("OpenViking is not configured; set ServerURL in system settings"),
	}
}

func (w *ProjectAwareWriter) List(ctx context.Context, projectID string) ([]ListItem, error) {
	client, _, err := w.clientForRead(ctx, projectID, "list")
	if err != nil {
		return nil, err
	}
	return client.List(ctx, projectID)
}

func (w *ProjectAwareWriter) Search(ctx context.Context, projectID string, query string, budget int, refsOnly bool) ([]SearchHit, error) {
	client, _, err := w.clientForRead(ctx, projectID, "search")
	if err != nil {
		return nil, err
	}
	return client.Search(ctx, projectID, query, budget, refsOnly)
}

func (w *ProjectAwareWriter) Read(ctx context.Context, projectID string, uri string) (ReadEntry, error) {
	client, _, err := w.clientForRead(ctx, projectID, "read")
	if err != nil {
		return ReadEntry{}, err
	}
	return client.Read(ctx, projectID, uri)
}

func (w *ProjectAwareWriter) Write(ctx context.Context, projectID string, input WriteInput) (WriteResult, error) {
	if w == nil || w.factory == nil {
		return WriteResult{}, errOpenVikingNotConfigured("write")
	}
	client, settings, err := w.factory.ProjectClient(ctx, projectID)
	if err != nil {
		return WriteResult{}, err
	}
	if !settings.loaded || client == nil {
		return WriteResult{}, errOpenVikingNotConfigured("write")
	}
	if !settings.EnableMemoryWrite {
		return WriteResult{Synced: false}, nil
	}
	if input.AutoSync && !settings.EnableAutoSync {
		return WriteResult{Synced: false}, nil
	}
	return client.Write(ctx, projectID, input)
}

func (w *ProjectAwareWriter) RegisterGitResource(ctx context.Context, projectID string, input GitResourceInput) (GitResourceResult, error) {
	if w == nil || w.factory == nil {
		return GitResourceResult{}, errOpenVikingNotConfigured("register_git_resource")
	}
	client, settings, err := w.factory.ProjectClient(ctx, projectID)
	if err != nil {
		return GitResourceResult{}, err
	}
	if !settings.loaded || client == nil {
		return GitResourceResult{}, errOpenVikingNotConfigured("register_git_resource")
	}
	return client.RegisterGitResource(ctx, input)
}

func (w *ProjectAwareWriter) ResourceSyncStatus(ctx context.Context, projectID string, query ResourceTaskQuery) (ResourceSyncStatus, error) {
	if w == nil || w.factory == nil {
		return ResourceSyncStatus{}, errOpenVikingNotConfigured("resource_sync_status")
	}
	client, settings, err := w.factory.ProjectClient(ctx, projectID)
	if err != nil {
		return ResourceSyncStatus{}, err
	}
	if !settings.loaded || client == nil {
		return ResourceSyncStatus{}, errOpenVikingNotConfigured("resource_sync_status")
	}
	return client.ResourceSyncStatus(ctx, query)
}

func (w *ProjectAwareWriter) clientForRead(ctx context.Context, projectID string, op string) (MemoryClient, Settings, error) {
	if w == nil || w.factory == nil {
		return nil, Settings{}, errOpenVikingNotConfigured(op)
	}
	client, settings, err := w.factory.ProjectClient(ctx, projectID)
	if err != nil {
		return nil, Settings{}, err
	}
	if !settings.loaded || client == nil {
		return nil, Settings{}, errOpenVikingNotConfigured(op)
	}
	return client, settings, nil
}
