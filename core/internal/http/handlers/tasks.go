package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http/middleware"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/tasks"
)

type TasksHandler struct {
	service *tasks.Service
}

func NewTasksHandler(service *tasks.Service) *TasksHandler {
	return &TasksHandler{service: service}
}

type createTaskRequest struct {
	Title               string   `json:"title"`
	Description         string   `json:"description"`
	Goal                string   `json:"goal"`
	Inputs              string   `json:"inputs"`
	Constraints         string   `json:"constraints"`
	ParentTaskID        string   `json:"parentTaskId"`
	Dependencies        []string `json:"dependencies"`
	DelegateToAgentID   string   `json:"delegateToAgentId"`
	DelegateToAgentType string   `json:"delegateToAgentType"`
	RequiredSkills      []string `json:"requiredSkills"`
	RequiredModel       string   `json:"requiredModel"`
	Priority            *int     `json:"priority"`
	OutputContract      string   `json:"outputContract"`
}

type updateTaskRequest struct {
	Title          *string  `json:"title"`
	Priority       *int     `json:"priority"`
	RequiredSkills []string `json:"requiredSkills"`
}

type delegateTaskRequest struct {
	AgentID   string `json:"agentId"`
	AgentType string `json:"agentType"`
	Reason    string `json:"reason"`
}

type cancelTaskRequest struct {
	Reason string `json:"reason"`
}

type startTaskRequest struct {
	RunID string `json:"runId"`
}

type heartbeatTaskRequest struct {
	RunID      string `json:"runId"`
	Checkpoint string `json:"checkpoint"`
}

type submitTaskRequest struct {
	RunID          string                `json:"runId"`
	ResultMarkdown string                `json:"resultMarkdown"`
	Artifacts      []submitArtifactInput `json:"artifacts"`
}

type submitArtifactInput struct {
	ArtifactType string         `json:"artifactType"`
	URI          string         `json:"uri"`
	Name         string         `json:"name"`
	Content      string         `json:"content"`
	Metadata     map[string]any `json:"metadata"`
}

type reviewTaskRequest struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

type failTaskRequest struct {
	RunID  string `json:"runId"`
	Reason string `json:"reason"`
}

type resumeTaskRequest struct {
	HandoffID string `json:"handoffId"`
	RunID     string `json:"runId"`
}

func (h *TasksHandler) ListTasks(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "TASK_SERVICE_UNAVAILABLE", "Task service unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() {
		if c.Query("assigneeAgentId") == actor.Agent.AgentID {
			if !actor.Agent.HasScope("task:read:own") {
				writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:read:own scope required", false, map[string]any{})
				return
			}
		} else if !actor.Agent.HasScope("task:read:delegated") && !actor.Agent.HasScope("task:read:tree") {
			writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:read:delegated scope required", false, map[string]any{})
			return
		}
	}
	items, err := h.service.List(c.Request.Context(), projectID, tasks.TaskFilters{
		Status:            c.Query("status"),
		AssigneeAgentID:   c.Query("assigneeAgentId"),
		AssigneeAgentType: c.Query("assigneeAgentType"),
		Skill:             c.Query("skill"),
		Q:                 c.Query("q"),
	})
	if err != nil {
		h.writeTaskError(c, projectID, "", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": tasksToJSON(items), "nextCursor": nil})
}

func (h *TasksHandler) GetTask(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "TASK_SERVICE_UNAVAILABLE", "Task service unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:read:own") && !actor.Agent.HasScope("task:read:delegated") && !actor.Agent.HasScope("task:read:tree") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:read scope required", false, map[string]any{})
		return
	}
	item, err := h.service.Get(c.Request.Context(), projectID, taskID)
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	c.JSON(http.StatusOK, taskToJSON(item))
}

func (h *TasksHandler) ListTaskEvents(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "TASK_SERVICE_UNAVAILABLE", "Task service unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:read:own") && !actor.Agent.HasScope("task:read:delegated") && !actor.Agent.HasScope("task:read:tree") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:read scope required", false, map[string]any{})
		return
	}
	items, nextCursor, err := h.service.ListTaskEvents(c.Request.Context(), projectID, taskID, parseIntQuery(c, "limit"), c.Query("before"))
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	result := make([]gin.H, 0, len(items))
	for _, item := range items {
		result = append(result, taskEventToJSON(item))
	}
	c.JSON(http.StatusOK, gin.H{"items": result, "nextCursor": nextCursor})
}

func (h *TasksHandler) CreateTask(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "TASK_SERVICE_UNAVAILABLE", "Task service unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:create") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:create scope required", false, map[string]any{})
		return
	}

	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}
	// 红线：忽略 body 中 delegateToAgentType，真实类型从 agentId 反查。
	item, err := h.service.Create(c.Request.Context(), actor, projectID, tasks.CreateTaskInput{
		Title:               req.Title,
		Description:         req.Description,
		Goal:                req.Goal,
		Inputs:              req.Inputs,
		Constraints:         req.Constraints,
		ParentTaskID:        req.ParentTaskID,
		Dependencies:        req.Dependencies,
		DelegateToAgentID:   req.DelegateToAgentID,
		DelegateToAgentType: "",
		RequiredSkills:      req.RequiredSkills,
		RequiredModel:       req.RequiredModel,
		Priority:            priority,
		OutputContract:      req.OutputContract,
	})
	if err != nil {
		h.writeTaskError(c, projectID, "", err)
		return
	}
	c.JSON(http.StatusOK, taskToJSON(item))
}

func (h *TasksHandler) UpdateTask(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "TASK_SERVICE_UNAVAILABLE", "Task service unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:update:delegated") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:update:delegated scope required", false, map[string]any{})
		return
	}

	var req updateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	item, err := h.service.Update(c.Request.Context(), actor, projectID, taskID, tasks.UpdateTaskInput{Title: req.Title, Priority: req.Priority, RequiredSkills: req.RequiredSkills})
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"taskId": item.TaskID, "status": item.Status, "updatedAt": item.UpdatedAt})
}

func (h *TasksHandler) DelegateTask(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "TASK_SERVICE_UNAVAILABLE", "Task service unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:delegate:codex") && !actor.Agent.HasScope("task:delegate:gemini") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:delegate:* scope required", false, map[string]any{})
		return
	}

	var req delegateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	// 红线：忽略 body 中 agentType，真实类型从 agentId 反查。
	item, err := h.service.Delegate(c.Request.Context(), actor, projectID, taskID, tasks.DelegateTaskInput{
		AgentID:   req.AgentID,
		AgentType: "",
		Reason:    req.Reason,
	})
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	c.JSON(http.StatusOK, taskToJSON(item))
}

func (h *TasksHandler) CancelTask(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "TASK_SERVICE_UNAVAILABLE", "Task service unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)

	var req cancelTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	item, err := h.service.Cancel(c.Request.Context(), actor, projectID, taskID, tasks.CancelTaskInput{Reason: req.Reason})
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"taskId": item.TaskID, "status": item.Status})
}

func (h *TasksHandler) StartTask(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:start:delegated") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:start:delegated scope required", false, map[string]any{})
		return
	}
	var req startTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	item, err := h.service.Start(c.Request.Context(), actor, projectID, taskID, tasks.StartTaskInput{RunID: req.RunID})
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	c.JSON(http.StatusOK, taskToJSON(item))
}

func (h *TasksHandler) HeartbeatTask(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:update:delegated") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:update:delegated scope required", false, map[string]any{})
		return
	}
	var req heartbeatTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	item, err := h.service.Heartbeat(c.Request.Context(), actor, projectID, taskID, tasks.HeartbeatTaskInput{RunID: req.RunID, Checkpoint: req.Checkpoint})
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	c.JSON(http.StatusOK, taskToJSON(item))
}

func (h *TasksHandler) SubmitTask(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:submit:delegated") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:submit:delegated scope required", false, map[string]any{})
		return
	}
	var req submitTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	artifacts := make([]tasks.SubmitArtifactInput, 0, len(req.Artifacts))
	for _, item := range req.Artifacts {
		artifacts = append(artifacts, tasks.SubmitArtifactInput{ArtifactType: item.ArtifactType, URI: item.URI, Name: item.Name, Content: item.Content, Metadata: item.Metadata})
	}
	item, err := h.service.Submit(c.Request.Context(), actor, projectID, taskID, tasks.SubmitTaskInput{RunID: req.RunID, ResultMarkdown: req.ResultMarkdown, Artifacts: artifacts})
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	c.JSON(http.StatusOK, taskToJSON(item))
}

func (h *TasksHandler) ReviewTask(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:review") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:review scope required", false, map[string]any{})
		return
	}
	var req reviewTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	item, err := h.service.Review(c.Request.Context(), actor, projectID, taskID, tasks.ReviewTaskInput{Approve: req.Approve, Reason: req.Reason})
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	c.JSON(http.StatusOK, taskToJSON(item))
}

func (h *TasksHandler) FailTask(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)
	var req failTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	item, err := h.service.Fail(c.Request.Context(), actor, projectID, taskID, tasks.FailTaskInput{RunID: req.RunID, Reason: req.Reason})
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	c.JSON(http.StatusOK, taskToJSON(item))
}

func (h *TasksHandler) ResumeTask(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	taskID := strings.TrimSpace(c.Param("taskId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("task:start:delegated") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "task:start:delegated scope required", false, map[string]any{})
		return
	}
	var req resumeTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	item, err := h.service.Resume(c.Request.Context(), actor, projectID, taskID, tasks.ResumeTaskInput{HandoffID: req.HandoffID, RunID: req.RunID})
	if err != nil {
		h.writeTaskError(c, projectID, taskID, err)
		return
	}
	c.JSON(http.StatusOK, taskToJSON(item))
}

func (h *TasksHandler) writeTaskError(c *gin.Context, projectID string, taskID string, err error) {
	var invalid *tasks.InvalidInputError
	if errors.As(err, &invalid) {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid task input", false, stringMapToAny(invalid.Details()))
		return
	}
	if errors.Is(err, tasks.ErrTaskNotFound) {
		writeAPIError(c, http.StatusNotFound, "TASK_NOT_FOUND", "Task not found", false, map[string]any{"projectId": projectID, "taskId": taskID})
		return
	}
	if errors.Is(err, tasks.ErrTaskNotEligibleForAgent) {
		writeAPIError(c, http.StatusConflict, "TASK_NOT_ELIGIBLE_FOR_AGENT", "Task is not eligible for target agent", false, map[string]any{})
		return
	}
	if errors.Is(err, tasks.ErrAgentNotBoundToProject) {
		writeAPIError(c, http.StatusConflict, "AGENT_NOT_BOUND_TO_PROJECT", "Agent is not bound to this project", false, map[string]any{})
		return
	}
	if errors.Is(err, tasks.ErrTaskAlreadyDelegated) {
		writeAPIError(c, http.StatusConflict, "TASK_ALREADY_DELEGATED", "Task already delegated", false, map[string]any{})
		return
	}
	if errors.Is(err, tasks.ErrTaskNotAssignedToCurrentAgent) {
		writeAPIError(c, http.StatusForbidden, "TASK_NOT_ASSIGNED_TO_CURRENT_AGENT", "Task is not assigned to current agent", false, map[string]any{})
		return
	}
	if errors.Is(err, tasks.ErrTaskActiveRunMismatch) {
		writeAPIError(c, http.StatusConflict, "TASK_ACTIVE_RUN_MISMATCH", "Task active run mismatch", false, map[string]any{})
		return
	}
	if errors.Is(err, tasks.ErrTaskDependencyNotDone) {
		writeAPIError(c, http.StatusConflict, "TASK_DEPENDENCY_NOT_DONE", "Task dependency not done", false, map[string]any{})
		return
	}
	if errors.Is(err, tasks.ErrTaskStatusInvalid) {
		writeAPIError(c, http.StatusConflict, "TASK_STATUS_INVALID", "Task status invalid", false, map[string]any{})
		return
	}
	if errors.Is(err, tasks.ErrHandoffNotFound) {
		writeAPIError(c, http.StatusNotFound, "HANDOFF_NOT_FOUND", "Handoff not found", false, map[string]any{})
		return
	}
	if errors.Is(err, tasks.ErrHandoffAlreadyConsumed) {
		writeAPIError(c, http.StatusConflict, "HANDOFF_ALREADY_CONSUMED", "Handoff already consumed", false, map[string]any{})
		return
	}
	if errors.Is(err, tasks.ErrProjectAccessDenied) {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "No access to project", false, map[string]any{"projectId": projectID})
		return
	}
	if errors.Is(err, tasks.ErrProjectArchived) {
		writeAPIError(c, http.StatusConflict, "PROJECT_ACCESS_DENIED", "Project is archived and read-only", false, map[string]any{"projectId": projectID, "status": "archived"})
		return
	}
	if errors.Is(err, tasks.ErrReviewScopeRequired) {
		writeAPIError(c, http.StatusForbidden, "TASK_NOT_ELIGIBLE_FOR_AGENT", "task:review scope required", false, map[string]any{})
		return
	}
	if errors.Is(err, tasks.ErrContextHandoffRequired) {
		writeAPIError(c, http.StatusConflict, "CONTEXT_HANDOFF_REQUIRED", "Context handoff required", false, map[string]any{})
		return
	}

	writeAPIError(c, http.StatusInternalServerError, "INTERNAL", "Internal server error", true, map[string]any{})
}

func tasksToJSON(items []tasks.TaskRecord) []gin.H {
	if len(items) == 0 {
		return []gin.H{}
	}
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		out = append(out, taskToJSON(item))
	}
	return out
}

func taskToJSON(item tasks.TaskRecord) gin.H {
	return gin.H{
		"taskId":                   item.TaskID,
		"projectId":                item.ProjectID,
		"title":                    item.Title,
		"description":              item.Description,
		"goal":                     nullableStringPointer(item.Goal),
		"inputs":                   nullableStringPointer(item.Inputs),
		"constraints":              nullableStringPointer(item.Constraints),
		"status":                   item.Status,
		"parentTaskId":             nullableStringPointer(item.ParentTaskID),
		"dependencies":             item.Dependencies,
		"assigneeAgentId":          nullableStringPointer(item.AssigneeAgentID),
		"assigneeAgentType":        nullableStringPointer(item.AssigneeAgentType),
		"delegatedByType":          defaultString(item.DelegatedByType, "system"),
		"delegatedByOperatorLabel": nullableStringPointer(item.DelegatedByOperatorLabel),
		"delegatedByAgentId":       nullableStringPointer(item.DelegatedByAgentID),
		"delegatedAt":              nullableTimePointer(item.DelegatedAt),
		"activeRunId":              nullableStringPointer(item.ActiveRunID),
		"lastHeartbeatAt":          nullableTimePointer(item.LastHeartbeatAt),
		"requiredSkills":           item.RequiredSkills,
		"requiredModel":            nullableStringPointer(item.RequiredModel),
		"outputContract":           nullableStringPointer(item.OutputContract),
		"priority":                 item.Priority,
		"createdAt":                item.CreatedAt,
		"updatedAt":                item.UpdatedAt,
	}
}

func taskEventToJSON(item tasks.TaskEvent) gin.H {
	return gin.H{
		"eventId":            item.EventID,
		"projectId":          item.ProjectID,
		"sessionId":          item.SessionID,
		"taskId":             nullableStringPointer(item.TaskID),
		"eventType":          item.EventType,
		"fromStatus":         nullableStringPointer(item.FromStatus),
		"toStatus":           nullableStringPointer(item.ToStatus),
		"actorType":          item.ActorType,
		"actorOperatorLabel": nullableStringPointer(item.ActorOperatorLabel),
		"actorAgentId":       nullableStringPointer(item.ActorAgentID),
		"actorRunId":         nullableStringPointer(item.ActorRunID),
		"payload":            item.Payload,
		"createdAt":          item.CreatedAt,
	}
}

func nullableStringPointer(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullableTimePointer(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parseIntQuery(c *gin.Context, key string) int {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return number
}
