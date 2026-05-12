package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http/middleware"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
)

func TestOpenVikingSystemSettingsGetDoesNotReturnAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	at := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	store := &openVikingSettingsStoreStub{
		systemSettings: openviking.Settings{
			ServerURL:         "http://ov:9090",
			EnableMemoryWrite: true,
			EnableAutoSync:    false,
			ApiKey:            "secret-key",
			ApiKeySet:         true,
			LastSyncAt:        &at,
			LastError:         "previous failure",
		},
	}
	router := gin.New()
	handler := NewOpenVikingSettingsHandler(store, nil)
	router.GET("/api/system/openviking/settings", handler.GetSystemSettings)

	req := httptest.NewRequest(http.MethodGet, "/api/system/openviking/settings", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if _, ok := body["apiKey"]; ok {
		t.Fatalf("response includes apiKey: %v", body)
	}
	if got, want := body["apiKeySet"], true; got != want {
		t.Fatalf("apiKeySet = %v, want %v", got, want)
	}
}

func TestOpenVikingSystemSettingsUpdateMapsValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &openVikingSettingsStoreStub{
		systemUpsertErr: &openviking.Error{Op: "settings_validate", Kind: openviking.ErrorKindBadRequest, Err: errors.New("serverUrl must include scheme and host")},
	}
	router := gin.New()
	handler := NewOpenVikingSettingsHandler(store, nil)
	router.PUT("/api/system/openviking/settings", handler.UpdateSystemSettings)

	req := httptest.NewRequest(http.MethodPut, "/api/system/openviking/settings", bytes.NewBufferString(`{
		"serverUrl":"ov:9090",
		"enableMemoryWrite":true,
		"enableAutoSync":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "INVALID_ARGUMENT"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
}

func TestOpenVikingSystemSettingsRejectsAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &openVikingSettingsStoreStub{}
	router := gin.New()
	router.Use(middleware.ResolveIdentity(middleware.ResolverOptions{
		ConsoleOperatorLabel: "console",
		Verifier: fakeVerifier{agent: identity.AgentIdentity{
			TokenID:   "tok_1",
			AgentID:   "agt_1",
			AgentType: "codex",
			Role:      "worker",
			Scopes:    []string{"project:admin"},
		}},
	}))
	handler := NewOpenVikingSettingsHandler(store, nil)
	router.GET("/api/system/openviking/settings", handler.GetSystemSettings)

	req := httptest.NewRequest(http.MethodGet, "/api/system/openviking/settings", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if store.systemGetCalls != 0 {
		t.Fatalf("systemGetCalls = %d, want 0", store.systemGetCalls)
	}
}

func TestOpenVikingSystemStatusCallsConfiguredHealthEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var authHeader, xAPIKeyHeader string
	ov := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("path = %q, want /health", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		xAPIKeyHeader = r.Header.Get("X-Api-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer ov.Close()

	store := &openVikingSettingsStoreStub{systemSettings: openviking.Settings{
		ServerURL:         ov.URL,
		EnableMemoryWrite: true,
		EnableAutoSync:    true,
		ApiKey:            "secret",
		ApiKeySet:         true,
	}}
	router := gin.New()
	handler := NewOpenVikingSettingsHandler(store, ov.Client())
	router.GET("/api/system/openviking/status", handler.GetSystemStatus)

	req := httptest.NewRequest(http.MethodGet, "/api/system/openviking/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if !body.OK {
		t.Fatalf("ok = false, want true; body=%s", rec.Body.String())
	}
	if got, want := authHeader, "Bearer secret"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
	if got, want := xAPIKeyHeader, "secret"; got != want {
		t.Fatalf("X-Api-Key = %q, want %q", got, want)
	}
}

func TestOpenVikingSystemStatusFallsBackToLegacyHealthz(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var calls []string
	ov := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/health":
			http.NotFound(w, r)
		case "/api/v1/system/status":
			http.NotFound(w, r)
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ov.Close()

	store := &openVikingSettingsStoreStub{systemSettings: openviking.Settings{
		ServerURL:         ov.URL,
		EnableMemoryWrite: true,
		EnableAutoSync:    true,
	}}
	router := gin.New()
	handler := NewOpenVikingSettingsHandler(store, ov.Client())
	router.GET("/api/system/openviking/status", handler.GetSystemStatus)

	req := httptest.NewRequest(http.MethodGet, "/api/system/openviking/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if !body.OK {
		t.Fatalf("ok = false, want true; body=%s", rec.Body.String())
	}
	if len(calls) < 3 {
		t.Fatalf("calls = %v, want at least 3 attempts", calls)
	}
}

func TestOpenVikingSystemStatusConsoleURLUsesNormalizedCandidate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var calls []string
	ov := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/ov-api/health":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ov.Close()

	store := &openVikingSettingsStoreStub{systemSettings: openviking.Settings{
		ServerURL:         ov.URL + "/console/",
		EnableMemoryWrite: true,
		EnableAutoSync:    true,
	}}
	router := gin.New()
	handler := NewOpenVikingSettingsHandler(store, ov.Client())
	router.GET("/api/system/openviking/status", handler.GetSystemStatus)

	req := httptest.NewRequest(http.MethodGet, "/api/system/openviking/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if !body.OK {
		t.Fatalf("ok = false, want true; body=%s", rec.Body.String())
	}
	if len(calls) == 0 || calls[0] != "/ov-api/health" {
		t.Fatalf("calls = %v, want first probe /ov-api/health", calls)
	}
}

type openVikingSettingsStoreStub struct {
	systemSettings  openviking.Settings
	systemGetErr    error
	systemUpsertErr error
	systemGetCalls  int
}

func (s *openVikingSettingsStoreStub) GetSystem(context.Context) (openviking.Settings, error) {
	s.systemGetCalls++
	if s.systemGetErr != nil {
		return openviking.Settings{}, s.systemGetErr
	}
	return s.systemSettings, nil
}

func (s *openVikingSettingsStoreStub) UpsertSystem(context.Context, openviking.UpsertSettingsInput, string) (openviking.Settings, error) {
	if s.systemUpsertErr != nil {
		return openviking.Settings{}, s.systemUpsertErr
	}
	return s.systemSettings, nil
}

type fakeVerifier struct {
	agent identity.AgentIdentity
	err   error
}

func (v fakeVerifier) VerifyToken(context.Context, string) (identity.AgentIdentity, error) {
	if v.err != nil {
		return identity.AgentIdentity{}, v.err
	}
	return v.agent, nil
}
