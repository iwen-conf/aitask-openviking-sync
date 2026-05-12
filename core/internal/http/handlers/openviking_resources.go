package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
)

type OpenVikingGitResourceRegistrar interface {
	RegisterGitResource(ctx context.Context, projectID string, input openviking.GitResourceInput) (openviking.GitResourceResult, error)
	ResourceSyncStatus(ctx context.Context, projectID string, query openviking.ResourceTaskQuery) (openviking.ResourceSyncStatus, error)
}

type OpenVikingResourcesHandler struct {
	registrar OpenVikingGitResourceRegistrar
}

func NewOpenVikingResourcesHandler(registrar OpenVikingGitResourceRegistrar) *OpenVikingResourcesHandler {
	return &OpenVikingResourcesHandler{registrar: registrar}
}

type openVikingGitResourceRequest struct {
	RepositoryURL string `json:"repositoryUrl"`
	TargetURI     string `json:"targetUri"`
	Reason        string `json:"reason"`
	WatchInterval int    `json:"watchInterval"`
	Wait          bool   `json:"wait"`
	Branch        string `json:"branch"`
	Commit        string `json:"commit"`
}

func (h *OpenVikingResourcesHandler) RegisterGitResource(c *gin.Context) {
	if h.registrar == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_WRITE_FAILED", "OpenViking resource registrar unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	if projectID == "" {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "projectId is required", false, map[string]any{"projectId": "cannot be empty"})
		return
	}
	var req openVikingGitResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.RepositoryURL) == "" {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "repositoryUrl is required", false, map[string]any{"repositoryUrl": "cannot be empty"})
		return
	}
	if strings.TrimSpace(req.TargetURI) == "" {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "targetUri is required", false, map[string]any{"targetUri": "cannot be empty"})
		return
	}
	if req.WatchInterval < 0 {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "watchInterval cannot be negative", false, map[string]any{"watchInterval": req.WatchInterval})
		return
	}

	result, err := h.registrar.RegisterGitResource(c.Request.Context(), projectID, openviking.GitResourceInput{
		RepositoryURL: strings.TrimSpace(req.RepositoryURL),
		TargetURI:     strings.TrimSpace(req.TargetURI),
		Reason:        strings.TrimSpace(req.Reason),
		WatchInterval: req.WatchInterval,
		Wait:          req.Wait,
		Branch:        strings.TrimSpace(req.Branch),
		Commit:        strings.TrimSpace(req.Commit),
	})
	if err != nil {
		writeOpenVikingResourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *OpenVikingResourcesHandler) GitResourceStatus(c *gin.Context) {
	if h.registrar == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_READ_FAILED", "OpenViking resource registrar unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	if projectID == "" {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "projectId is required", false, map[string]any{"projectId": "cannot be empty"})
		return
	}
	taskID := strings.TrimSpace(c.Query("taskId"))
	targetURI := firstNonEmptyString(c.Query("targetUri"), c.Query("uri"), "viking://resources/aitask")
	status, err := h.registrar.ResourceSyncStatus(c.Request.Context(), projectID, openviking.ResourceTaskQuery{
		TaskID:    taskID,
		TargetURI: targetURI,
		TaskType:  strings.TrimSpace(c.Query("taskType")),
		Status:    strings.TrimSpace(c.Query("status")),
		Limit:     parsePositiveInt(c.Query("limit"), 20),
	})
	if err != nil {
		writeOpenVikingStatusError(c, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

func writeOpenVikingResourceError(c *gin.Context, err error) {
	statusCode := http.StatusServiceUnavailable
	message := "OpenViking resource sync failed"
	var ovErr *openviking.Error
	if errors.As(err, &ovErr) {
		switch ovErr.Kind {
		case openviking.ErrorKindBadRequest:
			statusCode = http.StatusBadRequest
			message = ovErr.Error()
		case openviking.ErrorKindUnauthorized:
			statusCode = http.StatusForbidden
			message = ovErr.Error()
		case openviking.ErrorKindUnavailable:
			statusCode = http.StatusServiceUnavailable
		default:
			statusCode = http.StatusInternalServerError
		}
	}
	writeAPIError(c, statusCode, "OPENVIKING_WRITE_FAILED", message, statusCode >= 500, map[string]any{})
}

func writeOpenVikingStatusError(c *gin.Context, err error) {
	statusCode := http.StatusServiceUnavailable
	message := "OpenViking resource status read failed"
	var ovErr *openviking.Error
	if errors.As(err, &ovErr) {
		switch ovErr.Kind {
		case openviking.ErrorKindBadRequest:
			statusCode = http.StatusBadRequest
			message = ovErr.Error()
		case openviking.ErrorKindUnauthorized:
			statusCode = http.StatusForbidden
			message = ovErr.Error()
		case openviking.ErrorKindUnavailable:
			statusCode = http.StatusServiceUnavailable
		default:
			statusCode = http.StatusInternalServerError
		}
	}
	writeAPIError(c, statusCode, "OPENVIKING_READ_FAILED", message, statusCode >= 500, map[string]any{})
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func parsePositiveInt(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	n := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return fallback
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return fallback
	}
	return n
}
