package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/tasks"
)

type ArtifactsHandler struct {
	tasks *tasks.Service
}

func NewArtifactsHandler(tasksService *tasks.Service) *ArtifactsHandler {
	return &ArtifactsHandler{tasks: tasksService}
}

func (h *ArtifactsHandler) ListArtifacts(c *gin.Context) {
	if h.tasks == nil {
		writeAPIError(c, http.StatusInternalServerError, "TASK_SERVICE_UNAVAILABLE", "Task service unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	items, err := h.tasks.ListArtifacts(c.Request.Context(), projectID, c.Query("taskId"), c.Query("type"))
	if err != nil {
		h.writeArtifactError(c, projectID, "", err)
		return
	}

	response := make([]gin.H, 0, len(items))
	for _, item := range items {
		response = append(response, artifactToJSON(item, false))
	}
	c.JSON(http.StatusOK, gin.H{"items": response})
}

func (h *ArtifactsHandler) GetArtifact(c *gin.Context) {
	if h.tasks == nil {
		writeAPIError(c, http.StatusInternalServerError, "TASK_SERVICE_UNAVAILABLE", "Task service unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	artifactID := strings.TrimSpace(c.Param("artifactId"))
	item, err := h.tasks.GetArtifact(c.Request.Context(), projectID, artifactID)
	if err != nil {
		h.writeArtifactError(c, projectID, artifactID, err)
		return
	}
	c.JSON(http.StatusOK, artifactToJSON(item, true))
}

func (h *ArtifactsHandler) writeArtifactError(c *gin.Context, projectID string, artifactID string, err error) {
	var invalid *tasks.InvalidInputError
	if errors.As(err, &invalid) {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid artifact request", false, stringMapToAny(invalid.Details()))
		return
	}
	if errors.Is(err, tasks.ErrTaskNotFound) {
		writeAPIError(c, http.StatusNotFound, "TASK_NOT_FOUND", "Artifact not found", false, map[string]any{"projectId": projectID, "artifactId": artifactID})
		return
	}
	writeAPIError(c, http.StatusInternalServerError, "INTERNAL", "Internal server error", true, map[string]any{})
}

func artifactToJSON(item tasks.ArtifactSummary, includeContent bool) gin.H {
	result := gin.H{
		"artifactId":   item.ArtifactID,
		"taskId":       nullableStringPointer(item.TaskID),
		"artifactType": normalizeArtifactTypeForAPI(item.ArtifactType),
		"name":         item.Name,
		"path":         item.Path,
		"createdAt":    item.CreatedAt,
	}
	if includeContent {
		result["content"] = item.Content
		result["metadata"] = item.Metadata
	}
	return result
}

func normalizeArtifactTypeForAPI(value string) string {
	value = strings.TrimSpace(value)
	if value == "doc" {
		return "markdown"
	}
	if value == "" {
		return "other"
	}
	return value
}
