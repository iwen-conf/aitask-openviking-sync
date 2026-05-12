package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http/middleware"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
)

type OpenVikingSettingsStore interface {
	GetSystem(ctx context.Context) (openviking.Settings, error)
	UpsertSystem(ctx context.Context, input openviking.UpsertSettingsInput, actorID string) (openviking.Settings, error)
}

type OpenVikingSettingsHandler struct {
	store      OpenVikingSettingsStore
	httpClient *http.Client
}

func NewOpenVikingSettingsHandler(store OpenVikingSettingsStore, httpClient *http.Client) *OpenVikingSettingsHandler {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 2 * time.Second}
	}
	return &OpenVikingSettingsHandler{store: store, httpClient: httpClient}
}

type openVikingSettingsRequest struct {
	ServerURL         string  `json:"serverUrl"`
	EnableMemoryWrite bool    `json:"enableMemoryWrite"`
	EnableAutoSync    bool    `json:"enableAutoSync"`
	ApiKey            *string `json:"apiKey"`
}

func (h *OpenVikingSettingsHandler) assertSystemAdmin(c *gin.Context) error {
	actor := middleware.IdentityFromContext(c)
	if actor.IsOperator() {
		return nil
	}
	writeAPIError(c, http.StatusForbidden, "SYSTEM_ACCESS_DENIED", "operator credentials required", false, map[string]any{})
	return errors.New("operator credentials required")
}

func (h *OpenVikingSettingsHandler) GetSystemSettings(c *gin.Context) {
	if h.store == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_READ_FAILED", "OpenViking settings unavailable", true, map[string]any{})
		return
	}
	if err := h.assertSystemAdmin(c); err != nil {
		return
	}
	settings, err := h.store.GetSystem(c.Request.Context())
	if errors.Is(err, openviking.ErrSettingsNotFound) {
		c.JSON(http.StatusOK, settingsResponse(openviking.Settings{
			EnableMemoryWrite: true,
			EnableAutoSync:    true,
		}))
		return
	}
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "OPENVIKING_READ_FAILED", "OpenViking settings read failed", true, map[string]any{})
		return
	}
	c.JSON(http.StatusOK, settingsResponse(settings))
}

func (h *OpenVikingSettingsHandler) UpdateSystemSettings(c *gin.Context) {
	if h.store == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_WRITE_FAILED", "OpenViking settings unavailable", true, map[string]any{})
		return
	}
	if err := h.assertSystemAdmin(c); err != nil {
		return
	}

	var req openVikingSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	actorID := settingsActorID(middleware.IdentityFromContext(c))
	settings, err := h.store.UpsertSystem(c.Request.Context(), openviking.UpsertSettingsInput{
		ServerURL:         req.ServerURL,
		EnableMemoryWrite: req.EnableMemoryWrite,
		EnableAutoSync:    req.EnableAutoSync,
		ApiKey:            req.ApiKey,
	}, actorID)
	if err != nil {
		var ovErr *openviking.Error
		if errors.As(err, &ovErr) && ovErr.Kind == openviking.ErrorKindBadRequest {
			writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", ovErr.Error(), false, map[string]any{})
			return
		}
		writeAPIError(c, http.StatusInternalServerError, "OPENVIKING_WRITE_FAILED", "OpenViking settings write failed", true, map[string]any{})
		return
	}
	c.JSON(http.StatusOK, settingsResponse(settings))
}

func (h *OpenVikingSettingsHandler) GetSystemStatus(c *gin.Context) {
	if h.store == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_READ_FAILED", "OpenViking settings unavailable", true, map[string]any{})
		return
	}
	if err := h.assertSystemAdmin(c); err != nil {
		return
	}
	settings, err := h.store.GetSystem(c.Request.Context())
	if errors.Is(err, openviking.ErrSettingsNotFound) || (err == nil && strings.TrimSpace(settings.ServerURL) == "") {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "OpenViking settings not configured"})
		return
	}
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "OPENVIKING_READ_FAILED", "OpenViking settings read failed", true, map[string]any{})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	ok, latencyMs, errMsg := h.probeOpenViking(ctx, settings)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"ok": false, "latencyMs": latencyMs, "error": errMsg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "latencyMs": latencyMs})
}

func (h *OpenVikingSettingsHandler) probeOpenViking(ctx context.Context, settings openviking.Settings) (bool, int64, string) {
	baseCandidates, err := openviking.ServerURLCandidates(settings.ServerURL)
	if err != nil {
		return false, 0, err.Error()
	}

	targets := make([]string, 0, len(baseCandidates)*3)
	seen := map[string]struct{}{}
	for _, base := range baseCandidates {
		// Ordered by reliability:
		// 1) /health (new OpenViking public health)
		// 2) /api/v1/system/status (auth-aware system endpoint)
		// 3) /healthz (legacy/mock endpoint kept for backward compatibility)
		for _, endpoint := range []string{base + "/health", base + "/api/v1/system/status", base + "/healthz"} {
			if _, exists := seen[endpoint]; exists {
				continue
			}
			seen[endpoint] = struct{}{}
			targets = append(targets, endpoint)
		}
	}
	var lastErr string
	for _, target := range targets {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		if strings.TrimSpace(settings.ApiKey) != "" {
			token := strings.TrimSpace(settings.ApiKey)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Api-Key", token)
		}
		resp, err := h.httpClient.Do(req)
		latencyMs := time.Since(start).Milliseconds()
		if err != nil {
			lastErr = err.Error()
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
			return true, latencyMs, ""
		}
		lastErr = resp.Status
	}
	return false, 0, lastErr
}

func settingsResponse(settings openviking.Settings) gin.H {
	resp := gin.H{
		"serverUrl":         settings.ServerURL,
		"enableMemoryWrite": settings.EnableMemoryWrite,
		"enableAutoSync":    settings.EnableAutoSync,
		"apiKeySet":         settings.ApiKeySet,
	}
	if settings.LastSyncAt != nil {
		resp["lastSyncAt"] = settings.LastSyncAt.UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(settings.LastError) != "" {
		resp["lastError"] = settings.LastError
	}
	return resp
}

func settingsActorID(actor identity.Identity) string {
	if actor.IsAgent() {
		return actor.Agent.AgentID
	}
	if actor.IsOperator() {
		return actor.OperatorLabel
	}
	return "system"
}
