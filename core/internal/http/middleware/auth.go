package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/agents"
)

const identityContextKey = "aitask.identity"

type Verifier interface {
	VerifyToken(ctx context.Context, plainToken string) (identity.AgentIdentity, error)
}

type ResolverOptions struct {
	ConsoleOperatorLabel string
	Verifier             Verifier
}

func ResolveIdentity(opts ResolverOptions) gin.HandlerFunc {
	operatorLabel := strings.TrimSpace(opts.ConsoleOperatorLabel)
	if operatorLabel == "" {
		operatorLabel = "local-operator"
	}

	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header == "" {
			value := identity.Operator(operatorLabel)
			c.Set(identityContextKey, value)
			c.Next()
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(c, http.StatusUnauthorized, "AGENT_TOKEN_INVALID", "Agent token is invalid", false, map[string]any{})
			return
		}
		token := strings.TrimSpace(parts[1])
		if token == "" {
			writeError(c, http.StatusUnauthorized, "AGENT_TOKEN_INVALID", "Agent token is invalid", false, map[string]any{})
			return
		}
		if opts.Verifier == nil {
			writeError(c, http.StatusInternalServerError, "AGENT_AUTH_UNAVAILABLE", "Agent auth service unavailable", true, map[string]any{})
			return
		}

		agentIdentity, err := opts.Verifier.VerifyToken(c.Request.Context(), token)
		if err != nil {
			if err == agents.ErrAgentTokenExpired {
				writeError(c, http.StatusUnauthorized, "AGENT_TOKEN_EXPIRED", "Agent token expired", false, map[string]any{})
				return
			}
			writeError(c, http.StatusUnauthorized, "AGENT_TOKEN_INVALID", "Agent token is invalid", false, map[string]any{})
			return
		}
		value := identity.Identity{SenderType: identity.SenderTypeAgent, Agent: agentIdentity}
		c.Set(identityContextKey, value)
		c.Next()
	}
}

func RequireProjectAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		value := IdentityFromContext(c)
		if value.IsOperator() || value.IsSystem() {
			c.Next()
			return
		}
		projectID := strings.TrimSpace(c.Param("projectId"))
		if projectID == "" {
			writeError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "Missing projectId path parameter", false, map[string]any{})
			return
		}
		if !value.Agent.CanAccessProject(projectID) {
			writeError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "No access to this project", false, map[string]any{"projectId": projectID})
			return
		}
		c.Next()
	}
}

func IdentityFromContext(c *gin.Context) identity.Identity {
	if raw, ok := c.Get(identityContextKey); ok {
		if value, ok := raw.(identity.Identity); ok {
			return value
		}
	}
	return identity.Operator("local-operator")
}

func writeError(c *gin.Context, statusCode int, code string, message string, retriable bool, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	c.AbortWithStatusJSON(statusCode, gin.H{
		"code":      code,
		"message":   message,
		"retriable": retriable,
		"details":   details,
	})
}
