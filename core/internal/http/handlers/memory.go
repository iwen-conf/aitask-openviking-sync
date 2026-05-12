package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http/middleware"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
)

type MemoryHandler struct {
	client     openviking.MemoryClient
	db         *sql.DB
	fallbackNS string
}

func NewMemoryHandler(client openviking.MemoryClient, db *sql.DB, fallbackNamespace string) *MemoryHandler {
	return &MemoryHandler{client: client, db: db, fallbackNS: strings.TrimSpace(fallbackNamespace)}
}

// loadProjectNamespace returns the per-project namespace from the projects
// table, falling back to the env-time namespace when missing.
func (h *MemoryHandler) loadProjectNamespace(c *gin.Context, projectID string) string {
	if h.db == nil || strings.TrimSpace(projectID) == "" {
		return h.fallbackNS
	}
	var ns string
	if err := h.db.QueryRowContext(c.Request.Context(), `SELECT openviking_namespace FROM projects WHERE id = $1`, projectID).Scan(&ns); err != nil {
		return h.fallbackNS
	}
	if v := strings.TrimSpace(ns); v != "" {
		return v
	}
	return h.fallbackNS
}

type memoryWriteRequest struct {
	Target         string `json:"target"`
	Title          string `json:"title"`
	Content        string `json:"content"`
	RelatedTaskID  string `json:"relatedTaskId"`
	RelatedEventID string `json:"relatedEventId"`
	AutoSync       bool   `json:"autoSync"`
	TaskStatus     string `json:"task_status"`
	ActiveRunID    string `json:"active_run_id"`
	AgentToken     string `json:"agent_token"`
	AgentType      string `json:"agentType"`
}

func (h *MemoryHandler) ListMemory(c *gin.Context) {
	if h.client == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_READ_FAILED", "OpenViking client unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("memory:read") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "memory:read scope required", false, map[string]any{})
		return
	}
	items, err := h.client.List(c.Request.Context(), projectID)
	if err != nil {
		h.writeOpenVikingError(c, err, "OPENVIKING_READ_FAILED", "OpenViking read failed")
		return
	}
	root := fmt.Sprintf("viking://%s/projects/%s", h.loadProjectNamespace(c, projectID), projectID)
	c.JSON(http.StatusOK, gin.H{"root": root, "items": items})
}

func (h *MemoryHandler) SearchMemory(c *gin.Context) {
	if h.client == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_READ_FAILED", "OpenViking client unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("memory:search") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "memory:search scope required", false, map[string]any{})
		return
	}
	budget := 0
	if raw := strings.TrimSpace(c.Query("budget")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			budget = parsed
		}
	}
	refsOnly := strings.EqualFold(strings.TrimSpace(c.Query("refsOnly")), "true")
	items, err := h.client.Search(c.Request.Context(), projectID, c.Query("q"), budget, refsOnly)
	if err != nil {
		h.writeOpenVikingError(c, err, "OPENVIKING_READ_FAILED", "OpenViking search failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *MemoryHandler) ReadMemory(c *gin.Context) {
	if h.client == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_READ_FAILED", "OpenViking client unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("memory:read") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "memory:read scope required", false, map[string]any{})
		return
	}
	uri := strings.TrimSpace(c.Query("uri"))
	if uri == "" {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "uri query is required", false, map[string]any{"uri": "cannot be empty"})
		return
	}
	item, err := h.client.Read(c.Request.Context(), projectID, uri)
	if err != nil {
		h.writeOpenVikingError(c, err, "OPENVIKING_READ_FAILED", "OpenViking read failed")
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *MemoryHandler) WriteMemory(c *gin.Context) {
	if h.client == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_WRITE_FAILED", "OpenViking client unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	if err := h.assertProjectWritable(c, projectID); err != nil {
		return
	}
	var req memoryWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.TaskStatus) != "" || strings.TrimSpace(req.ActiveRunID) != "" || strings.TrimSpace(req.AgentToken) != "" {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "Memory write payload contains forbidden authority fields", false, map[string]any{
			"forbidden": []string{"task_status", "active_run_id", "agent_token"},
		})
		return
	}
	// 红线：身份由 token/middleware 决定，忽略 body 中 agentType。
	req.AgentType = ""
	actor := middleware.IdentityFromContext(c)
	target := strings.TrimSpace(strings.ToLower(req.Target))
	if (target == "decisions" || target == "summary") && !actor.IsAgent() {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "Only agent identity can write decisions/summary", false, map[string]any{})
		return
	}
	if actor.IsAgent() {
		if (target == "decisions" || target == "decision") && !actor.Agent.HasScope("memory:write:decision") && !actor.Agent.HasScope("memory:write") {
			writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "memory:write:decision scope required", false, map[string]any{})
			return
		}
		if (target == "summary" || target == "note" || target == "report") && !actor.Agent.HasScope("memory:write:summary") && !actor.Agent.HasScope("memory:write") {
			writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "memory:write:summary scope required", false, map[string]any{})
			return
		}
		if (target == "handoff" || target == "handoffs") && !actor.Agent.HasScope("memory:write:handoff") && !actor.Agent.HasScope("memory:write") {
			writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "memory:write:handoff scope required", false, map[string]any{})
			return
		}
	}
	result, err := h.client.Write(c.Request.Context(), projectID, openviking.WriteInput{Target: req.Target, Title: req.Title, Content: req.Content, RelatedTaskID: req.RelatedTaskID, RelatedEventID: req.RelatedEventID, AutoSync: req.AutoSync})
	if err != nil {
		h.writeOpenVikingError(c, err, "OPENVIKING_WRITE_FAILED", "OpenViking write failed")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *MemoryHandler) ListSkills(c *gin.Context) {
	if h.client == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_READ_FAILED", "OpenViking client unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("memory:read") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "memory:read scope required", false, map[string]any{})
		return
	}
	items, err := h.client.List(c.Request.Context(), projectID)
	if err != nil {
		h.writeOpenVikingError(c, err, "OPENVIKING_READ_FAILED", "OpenViking read failed")
		return
	}
	skills := make([]gin.H, 0)
	for _, item := range items {
		uri := strings.TrimSpace(item.URI)
		if !strings.Contains(uri, "/skills/") || !strings.HasSuffix(uri, ".md") {
			continue
		}
		name := strings.TrimSuffix(uri[strings.LastIndex(uri, "/")+1:], ".md")
		skills = append(skills, gin.H{
			"name":  name,
			"title": item.Title,
			"uri":   item.URI,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": skills})
}

func (h *MemoryHandler) ShowSkill(c *gin.Context) {
	if h.client == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "OPENVIKING_READ_FAILED", "OpenViking client unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("memory:read") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "memory:read scope required", false, map[string]any{})
		return
	}
	skillName := strings.TrimSpace(c.Param("skillName"))
	if skillName == "" {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "skillName is required", false, map[string]any{"skillName": "cannot be empty"})
		return
	}
	uri := fmt.Sprintf("viking://%s/projects/%s/skills/%s.md", h.loadProjectNamespace(c, projectID), projectID, skillName)
	entry, err := h.client.Read(c.Request.Context(), projectID, uri)
	if err != nil {
		h.writeOpenVikingError(c, err, "OPENVIKING_READ_FAILED", "OpenViking read failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"name":        skillName,
		"uri":         entry.URI,
		"title":       entry.Title,
		"contentType": entry.ContentType,
		"content":     entry.Content,
	})
}

func (h *MemoryHandler) writeOpenVikingError(c *gin.Context, err error, code string, fallbackMessage string) {
	statusCode := http.StatusServiceUnavailable
	message := fallbackMessage
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
	retriable := statusCode >= 500
	writeAPIError(c, statusCode, code, message, retriable, map[string]any{})
}

func (h *MemoryHandler) assertProjectWritable(c *gin.Context, projectID string) error {
	if h.db == nil {
		return nil
	}
	var status string
	err := h.db.QueryRowContext(c.Request.Context(), `SELECT status FROM projects WHERE id = $1`, projectID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		writeAPIError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found", false, map[string]any{"projectId": projectID})
		return err
	}
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "INTERNAL", "Internal server error", true, map[string]any{})
		return err
	}
	if strings.TrimSpace(status) == "archived" {
		writeAPIError(c, http.StatusConflict, "PROJECT_ACCESS_DENIED", "Project is archived and read-only", false, map[string]any{"projectId": projectID, "status": "archived"})
		return errors.New("project archived")
	}
	return nil
}
