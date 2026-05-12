package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	agentsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/agents"
)

type AgentsHandler struct {
	service *agentsvc.Service
}

func NewAgentsHandler(service *agentsvc.Service) *AgentsHandler {
	return &AgentsHandler{service: service}
}

type createAgentRequest struct {
	Name         string   `json:"name"`
	AgentType    string   `json:"agentType"`
	Role         string   `json:"role"`
	DefaultModel string   `json:"defaultModel"`
	Skills       []string `json:"skills"`
	Models       []string `json:"models"`
}

type issueTokenRequest struct {
	ExpiresAt string   `json:"expiresAt"`
	Scopes    []string `json:"scopes"`
}

type revokeTokenRequest struct {
	Reason string `json:"reason"`
}

type bindAgentRequest struct {
	Role    string `json:"role"`
	Enabled bool   `json:"enabled"`
}

func (h *AgentsHandler) ListAgents(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "AGENT_SERVICE_UNAVAILABLE", "Agent service unavailable", true, map[string]any{})
		return
	}
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "AGENT_LIST_FAILED", "Failed to list agents", true, map[string]any{})
		return
	}
	response := make([]gin.H, 0, len(items))
	for _, item := range items {
		response = append(response, gin.H{
			"agentId":       item.AgentID,
			"name":          item.Name,
			"agentType":     item.AgentType,
			"role":          item.Role,
			"status":        item.Status,
			"scopes":        item.Scopes,
			"defaultModel":  item.DefaultModel,
			"models":        item.Models,
			"skills":        item.Skills,
			"boundProjects": item.BoundProjects,
			"tokens":        tokenSummariesToJSON(item.Tokens),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": response})
}

func (h *AgentsHandler) CreateAgent(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "AGENT_SERVICE_UNAVAILABLE", "Agent service unavailable", true, map[string]any{})
		return
	}
	var req createAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	item, err := h.service.Create(c.Request.Context(), agentsvc.CreateAgentInput{
		Name:         req.Name,
		AgentType:    req.AgentType,
		Role:         req.Role,
		DefaultModel: req.DefaultModel,
		Skills:       req.Skills,
		Models:       req.Models,
	})
	if err != nil {
		h.writeAgentError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"agentId":      item.AgentID,
		"name":         item.Name,
		"agentType":    item.AgentType,
		"role":         item.Role,
		"status":       item.Status,
		"scopes":       item.Scopes,
		"defaultModel": item.DefaultModel,
		"skills":       item.Skills,
		"models":       item.Models,
	})
}

func (h *AgentsHandler) IssueToken(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "AGENT_SERVICE_UNAVAILABLE", "Agent service unavailable", true, map[string]any{})
		return
	}
	agentID := strings.TrimSpace(c.Param("agentId"))
	var req issueTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	var expiresAt *time.Time
	if raw := strings.TrimSpace(req.ExpiresAt); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "expiresAt must be RFC3339 datetime", false, map[string]any{"expiresAt": raw})
			return
		}
		expiresAt = &value
	}

	result, err := h.service.IssueToken(c.Request.Context(), agentID, agentsvc.IssueTokenInput{ExpiresAt: expiresAt, Scopes: req.Scopes})
	if err != nil {
		h.writeAgentError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"tokenId": result.TokenID, "agentToken": result.AgentToken, "expiresAt": result.ExpiresAt})
}

func (h *AgentsHandler) RevokeToken(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "AGENT_SERVICE_UNAVAILABLE", "Agent service unavailable", true, map[string]any{})
		return
	}
	agentID := strings.TrimSpace(c.Param("agentId"))
	tokenID := strings.TrimSpace(c.Param("tokenId"))
	var req revokeTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	result, err := h.service.RevokeToken(c.Request.Context(), agentID, tokenID, req.Reason)
	if err != nil {
		h.writeAgentError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tokenId": result.TokenID, "revokedAt": result.RevokedAt})
}

func (h *AgentsHandler) BindProject(c *gin.Context) {
	if h.service == nil {
		writeAPIError(c, http.StatusInternalServerError, "AGENT_SERVICE_UNAVAILABLE", "Agent service unavailable", true, map[string]any{})
		return
	}
	projectID := strings.TrimSpace(c.Param("projectId"))
	agentID := strings.TrimSpace(c.Param("agentId"))
	var req bindAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	result, err := h.service.BindProject(c.Request.Context(), projectID, agentID, agentsvc.BindInput{Role: req.Role, Enabled: req.Enabled})
	if err != nil {
		h.writeAgentError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"projectId": result.ProjectID, "agentId": result.AgentID, "role": result.Role, "enabled": result.Enabled})
}

func (h *AgentsHandler) writeAgentError(c *gin.Context, err error) {
	var invalid *agentsvc.InvalidInputError
	if errors.As(err, &invalid) {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid agent input", false, stringMapToAny(invalid.Details()))
		return
	}
	if errors.Is(err, agentsvc.ErrAgentNotFound) {
		writeAPIError(c, http.StatusNotFound, "AGENT_NOT_FOUND", "Agent not found", false, map[string]any{})
		return
	}
	if errors.Is(err, agentsvc.ErrAgentTokenInvalid) {
		writeAPIError(c, http.StatusUnauthorized, "AGENT_TOKEN_INVALID", "Agent token invalid", false, map[string]any{})
		return
	}
	if errors.Is(err, agentsvc.ErrAgentTokenExpired) {
		writeAPIError(c, http.StatusUnauthorized, "AGENT_TOKEN_EXPIRED", "Agent token expired", false, map[string]any{})
		return
	}
	if errors.Is(err, agentsvc.ErrProjectArchived) {
		writeAPIError(c, http.StatusConflict, "PROJECT_ACCESS_DENIED", "Project is archived and read-only", false, map[string]any{"status": "archived"})
		return
	}
	writeAPIError(c, http.StatusInternalServerError, "INTERNAL", "Internal server error", true, map[string]any{})
}

func tokenSummariesToJSON(items []agentsvc.AgentTokenSummary) []gin.H {
	if len(items) == 0 {
		return []gin.H{}
	}
	out := make([]gin.H, 0, len(items))
	for _, item := range items {
		outItem := gin.H{"tokenId": item.TokenID, "expiresAt": item.ExpiresAt, "scopes": item.Scopes, "revokedAt": nil}
		if item.RevokedAt != nil {
			outItem["revokedAt"] = *item.RevokedAt
		}
		out = append(out, outItem)
	}
	return out
}
