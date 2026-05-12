package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http/middleware"
	contextsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/context"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/room"
)

type ContextHandler struct {
	service    *contextsvc.Service
	projects   ProjectService
	rooms      *room.Service
	openviking openviking.MemoryClient
}

func NewContextHandler(service *contextsvc.Service, projects ProjectService, rooms *room.Service, openvikingClient openviking.MemoryClient) *ContextHandler {
	return &ContextHandler{service: service, projects: projects, rooms: rooms, openviking: openvikingClient}
}

type contextReportRequest struct {
	RunID                string `json:"runId"`
	ReportedInputTokens  int    `json:"reportedInputTokens"`
	ReportedOutputTokens int    `json:"reportedOutputTokens"`
	MaxContextTokens     int    `json:"maxContextTokens"`
	Source               string `json:"source"`
}

type createHandoffRequest struct {
	TaskID          string `json:"taskId"`
	Reason          string `json:"reason"`
	HandoffMarkdown string `json:"handoffMarkdown"`
}

func (h *ContextHandler) Report(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	var req contextReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	output, err := h.service.Report(c.Request.Context(), actor, contextsvc.ReportInput{
		ProjectID:            projectID,
		RunID:                req.RunID,
		ReportedInputTokens:  req.ReportedInputTokens,
		ReportedOutputTokens: req.ReportedOutputTokens,
		MaxContextTokens:     req.MaxContextTokens,
		Source:               req.Source,
	})
	if err != nil {
		h.writeContextError(c, projectID, err)
		return
	}
	c.JSON(http.StatusOK, output)
}

func (h *ContextHandler) CreateHandoff(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	var req createHandoffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	output, err := h.service.CreateHandoff(c.Request.Context(), actor, contextsvc.CreateHandoffInput{
		ProjectID:       projectID,
		TaskID:          req.TaskID,
		Reason:          req.Reason,
		HandoffMarkdown: req.HandoffMarkdown,
	})
	if err != nil {
		h.writeContextError(c, projectID, err)
		return
	}
	c.JSON(http.StatusOK, output)
}

func (h *ContextHandler) GetCurrentHandoff(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	output, err := h.service.GetCurrentHandoff(c.Request.Context(), actor, projectID)
	if err != nil {
		h.writeContextError(c, projectID, err)
		return
	}
	c.JSON(http.StatusOK, output)
}

func (h *ContextHandler) Bootstrap(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)

	project, err := h.projects.Get(c.Request.Context(), projectID)
	if err != nil {
		writeAPIError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found", false, map[string]any{"projectId": projectID})
		return
	}

	runID, budget, err := h.service.FindActiveRunBudget(c.Request.Context(), actor, projectID)
	if err != nil {
		h.writeContextError(c, projectID, err)
		return
	}

	contextRefs := make([]gin.H, 0)
	if h.openviking != nil {
		if items, listErr := h.openviking.List(c.Request.Context(), projectID); listErr == nil {
			for i, item := range items {
				if i >= 80 {
					break
				}
				contextRefs = append(contextRefs, gin.H{
					"uri":             item.URI,
					"title":           item.Title,
					"estimatedTokens": 300,
				})
			}
		}
	}

	roomID := project.RoomID
	if roomID == "" && h.rooms != nil {
		if snapshot, roomErr := h.rooms.GetRoom(c.Request.Context(), projectID); roomErr == nil {
			roomID = snapshot.RoomID
		}
	}
	unreadMentions := 0
	if h.rooms != nil {
		if count, countErr := h.rooms.UnreadMentions(c.Request.Context(), actor, projectID); countErr == nil {
			unreadMentions = count
		}
	}

	nextAction := contextsvc.NextAction{Type: "read_delegated_task", Message: "Run aitask task current", Command: "aitask task current"}
	if pending, pendingErr := h.service.HasPendingHandoff(c.Request.Context(), actor, projectID); pendingErr == nil && pending {
		nextAction = contextsvc.NextAction{Type: "handoff_current", Message: "Run aitask context handoff current", Command: "aitask context handoff current"}
	}

	identityJSON := gin.H{"agentId": nil, "agentType": nil, "role": nil}
	if actor.IsAgent() {
		identityJSON = gin.H{
			"agentId":   actor.Agent.AgentID,
			"agentType": actor.Agent.AgentType,
			"role":      actor.Agent.Role,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"identity": identityJSON,
		"project": gin.H{
			"projectId": project.ProjectID,
			"name":      project.Name,
			"status":    project.Status,
		},
		"session": gin.H{
			"sessionId": project.ActiveSessionID,
			"status":    "active",
		},
		"run": gin.H{
			"runId": runID,
			"contextBudget": gin.H{
				"maxContextTokens":    budget.MaxContextTokens,
				"estimatedUsedTokens": budget.EstimatedUsedTokens,
				"state":               budget.State,
				"usageRatio":          budget.UsageRatio,
			},
		},
		"contextRefs": contextRefs,
		"room": gin.H{
			"roomId":         roomID,
			"recentSummary":  "",
			"unreadMentions": unreadMentions,
		},
		"nextAction": gin.H{
			"type":    nextAction.Type,
			"message": nextAction.Message,
			"command": nextAction.Command,
		},
	})
}

func (h *ContextHandler) writeContextError(c *gin.Context, projectID string, err error) {
	if details, ok := contextsvc.IsInvalidInput(err); ok {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid context request", false, stringMapToAny(details))
		return
	}
	if errors.Is(err, contextsvc.ErrContextRunAccessDenied) {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "No access to project context", false, map[string]any{"projectId": projectID})
		return
	}
	if errors.Is(err, contextsvc.ErrContextRunNotFound) {
		writeAPIError(c, http.StatusNotFound, "TASK_ACTIVE_RUN_MISMATCH", "Active run not found", false, map[string]any{})
		return
	}
	if errors.Is(err, contextsvc.ErrContextHandoffNotFound) {
		writeAPIError(c, http.StatusNotFound, "HANDOFF_NOT_FOUND", "Handoff not found", false, map[string]any{})
		return
	}
	if errors.Is(err, contextsvc.ErrContextHandoffRequired) {
		writeAPIError(c, http.StatusConflict, "CONTEXT_HANDOFF_REQUIRED", "Context handoff required", false, map[string]any{})
		return
	}
	if errors.Is(err, contextsvc.ErrProjectArchived) {
		writeAPIError(c, http.StatusConflict, "PROJECT_ACCESS_DENIED", "Project is archived and read-only", false, map[string]any{"projectId": projectID, "status": "archived"})
		return
	}
	if errors.Is(err, contextsvc.ErrContextBudgetExceeded) {
		writeAPIError(c, http.StatusConflict, "CONTEXT_BUDGET_EXCEEDED", "Context budget exceeded", false, map[string]any{})
		return
	}
	writeAPIError(c, http.StatusInternalServerError, "INTERNAL", "Internal server error", true, map[string]any{})
}
