package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http/middleware"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
	projectsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/projects"
)

type ProjectService interface {
	Create(ctx context.Context, input projectsvc.CreateProjectInput) (projectsvc.CreateProjectOutput, error)
	List(ctx context.Context) (projectsvc.ListProjectsOutput, error)
	Get(ctx context.Context, projectID string) (projectsvc.ProjectDetail, error)
	Update(ctx context.Context, projectID string, input projectsvc.UpdateProjectInput) (projectsvc.UpdateProjectOutput, error)
	Complete(ctx context.Context, projectID string, input projectsvc.CompleteProjectInput) (projectsvc.CompleteProjectOutput, error)
	Archive(ctx context.Context, projectID string, input projectsvc.ArchiveProjectInput) (projectsvc.ArchiveProjectOutput, error)
}

type ProjectsHandler struct {
	service ProjectService
}

func NewProjectsHandler(service ProjectService) *ProjectsHandler {
	return &ProjectsHandler{service: service}
}

type createProjectRequest struct {
	Name        string `json:"name"`
	Goal        string `json:"goal"`
	Description string `json:"description"`
}

type updateProjectRequest struct {
	Name                  *string                           `json:"name"`
	Goal                  *string                           `json:"goal"`
	Description           *string                           `json:"description"`
	CompletionPolicy      *projectsvc.CompletionPolicyPatch `json:"completionPolicy"`
	OpenVikingNamespace   *string                           `json:"openvikingNamespace"`
	OpenVikingWorkspaceID *string                           `json:"openvikingWorkspaceId"`
}

type completeProjectRequest struct {
	Confirm bool `json:"confirm"`
}

type archiveProjectRequest struct {
	Confirm bool   `json:"confirm"`
	Reason  string `json:"reason"`
}

func (h *ProjectsHandler) ListProjects(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "PROJECT_SERVICE_UNAVAILABLE", "Project service is unavailable", true, map[string]any{})
		return
	}

	output, err := h.service.List(c.Request.Context())
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "PROJECT_LIST_FAILED", "Failed to list projects", true, map[string]any{})
		return
	}

	items := make([]gin.H, 0, len(output.Items))
	for _, item := range output.Items {
		items = append(items, gin.H{
			"projectId": item.ProjectID,
			"name":      item.Name,
			"status":    item.Status,
			"progress": gin.H{
				"done":    item.Progress.Done,
				"total":   item.Progress.Total,
				"blocked": item.Progress.Blocked,
			},
			"updatedAt": item.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items":      items,
		"nextCursor": output.NextCursor,
	})
}

func (h *ProjectsHandler) GetProject(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "PROJECT_SERVICE_UNAVAILABLE", "Project service is unavailable", true, map[string]any{})
		return
	}

	projectID := strings.TrimSpace(c.Param("projectId"))
	output, err := h.service.Get(c.Request.Context(), projectID)
	if err != nil {
		h.writeProjectError(c, projectID, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projectId":             output.ProjectID,
		"name":                  output.Name,
		"goal":                  output.Goal,
		"description":           output.Description,
		"status":                output.Status,
		"activeSessionId":       output.ActiveSessionID,
		"openvikingRoot":        output.OpenVikingRoot,
		"openvikingNamespace":   output.OpenVikingNamespace,
		"openvikingWorkspaceId": output.OpenVikingWorkspaceID,
		"roomId":                output.RoomID,
		"operatorLabel":         output.OperatorLabel,
		"completionPolicy": gin.H{
			"requiredTasks": output.CompletionPolicy.RequiredTasks,
			"blockedTasks":  output.CompletionPolicy.BlockedTasks,
			"failedTasks":   output.CompletionPolicy.FailedTasks,
			"reviewPolicy":  output.CompletionPolicy.ReviewPolicy,
		},
		"createdAt": output.CreatedAt,
		"updatedAt": output.UpdatedAt,
	})
}

func (h *ProjectsHandler) CreateProject(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "PROJECT_SERVICE_UNAVAILABLE", "Project service is unavailable", true, map[string]any{})
		return
	}

	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	output, err := h.service.Create(c.Request.Context(), projectsvc.CreateProjectInput{
		Name:        req.Name,
		Goal:        req.Goal,
		Description: req.Description,
	})
	if err != nil {
		slog.Error("create project failed", "error", err)
		h.writeProjectError(c, "", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projectId":       output.ProjectID,
		"name":            output.Name,
		"status":          output.Status,
		"activeSessionId": output.ActiveSessionID,
		"openvikingRoot":  output.OpenVikingRoot,
		"roomId":          output.RoomID,
		"initCommand":     output.InitCommand,
	})
}

func (h *ProjectsHandler) UpdateProject(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "PROJECT_SERVICE_UNAVAILABLE", "Project service is unavailable", true, map[string]any{})
		return
	}

	projectID := strings.TrimSpace(c.Param("projectId"))
	var req updateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	output, err := h.service.Update(c.Request.Context(), projectID, projectsvc.UpdateProjectInput{
		Name:                  req.Name,
		Goal:                  req.Goal,
		Description:           req.Description,
		CompletionPolicy:      req.CompletionPolicy,
		OpenVikingNamespace:   req.OpenVikingNamespace,
		OpenVikingWorkspaceID: req.OpenVikingWorkspaceID,
	})
	if err != nil {
		h.writeProjectError(c, projectID, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projectId": output.ProjectID,
		"name":      output.Name,
		"status":    output.Status,
		"updatedAt": output.UpdatedAt,
	})
}

func (h *ProjectsHandler) CompleteProject(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "PROJECT_SERVICE_UNAVAILABLE", "Project service is unavailable", true, map[string]any{})
		return
	}

	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:complete:project") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:complete:project scope required", false, map[string]any{"projectId": projectID})
		return
	}
	var req completeProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	output, err := h.service.Complete(c.Request.Context(), projectID, projectsvc.CompleteProjectInput{Confirm: req.Confirm})
	if err != nil {
		h.writeProjectError(c, projectID, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projectId": output.ProjectID,
		"status":    output.Status,
		"policyResult": gin.H{
			"passed":      output.PolicyResult.Passed,
			"failedItems": completionPolicyItemsToJSON(output.PolicyResult.FailedItems),
		},
	})
}

func (h *ProjectsHandler) ArchiveProject(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "PROJECT_SERVICE_UNAVAILABLE", "Project service is unavailable", true, map[string]any{})
		return
	}

	projectID := strings.TrimSpace(c.Param("projectId"))
	var req archiveProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	output, err := h.service.Archive(c.Request.Context(), projectID, projectsvc.ArchiveProjectInput{
		Confirm: req.Confirm,
		Reason:  req.Reason,
	})
	if err != nil {
		h.writeProjectError(c, projectID, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projectId": output.ProjectID,
		"status":    output.Status,
		"updatedAt": output.UpdatedAt,
	})
}

func (h *ProjectsHandler) writeProjectError(c *gin.Context, projectID string, err error) {
	var invalid *projectsvc.InvalidInputError
	if errors.As(err, &invalid) {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid project input", false, stringMapToAny(invalid.Details()))
		return
	}

	var completionFailed *projectsvc.CompletionPolicyFailedError
	if errors.As(err, &completionFailed) {
		writeAPIError(c, http.StatusConflict, "PROJECT_COMPLETION_POLICY_FAILED", "Project completion policy check failed", false, map[string]any{
			"failedItems": completionPolicyItemsToJSON(completionFailed.FailedItems()),
		})
		return
	}

	if errors.Is(err, projectsvc.ErrProjectNotFound) {
		writeAPIError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found", false, map[string]any{"projectId": projectID})
		return
	}
	if errors.Is(err, projectsvc.ErrProjectArchived) {
		writeAPIError(c, http.StatusConflict, "PROJECT_ACCESS_DENIED", "Project is archived and read-only", false, map[string]any{"projectId": projectID, "status": "archived"})
		return
	}

	if errors.Is(err, projectsvc.ErrCreateProjectFailed) {
		message := "Failed to create project"
		var ovErr *openviking.Error
		if errors.As(err, &ovErr) {
			message = ovErr.Error()
		}
		writeAPIError(c, http.StatusInternalServerError, "PROJECT_CREATE_FAILED", message, true, map[string]any{})
		return
	}

	writeAPIError(c, http.StatusInternalServerError, "INTERNAL", "Internal server error", true, map[string]any{})
}

func stringMapToAny(input map[string]string) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func completionPolicyItemsToJSON(items []projectsvc.CompletionPolicyResultItem) []map[string]any {
	if len(items) == 0 {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		line := map[string]any{
			"code":    item.Code,
			"message": item.Message,
		}
		if strings.TrimSpace(item.TaskID) != "" {
			line["taskId"] = item.TaskID
		}
		result = append(result, line)
	}
	return result
}
