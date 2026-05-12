package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
	agentsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/agents"
)

type verifierStub struct {
	identity  identity.AgentIdentity
	err       error
	lastToken string
}

func (s *verifierStub) VerifyToken(_ context.Context, plainToken string) (identity.AgentIdentity, error) {
	s.lastToken = plainToken
	return s.identity, s.err
}

func TestResolveIdentityUsesOperatorWhenAuthorizationMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(ResolveIdentity(ResolverOptions{
		ConsoleOperatorLabel: "console-operator",
	}))
	router.GET("/identity", func(c *gin.Context) {
		value := IdentityFromContext(c)
		c.JSON(http.StatusOK, gin.H{
			"senderType":    string(value.SenderType),
			"operatorLabel": value.OperatorLabel,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/identity", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		SenderType    string `json:"senderType"`
		OperatorLabel string `json:"operatorLabel"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := body.SenderType, "operator"; got != want {
		t.Fatalf("senderType = %q, want %q", got, want)
	}
	if got, want := body.OperatorLabel, "console-operator"; got != want {
		t.Fatalf("operatorLabel = %q, want %q", got, want)
	}
}

func TestResolveIdentityRejectsMalformedBearerHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(ResolveIdentity(ResolverOptions{
		ConsoleOperatorLabel: "console-operator",
	}))
	router.GET("/identity", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/identity", nil)
	req.Header.Set("Authorization", "Token abc")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := body.Code, "AGENT_TOKEN_INVALID"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
}

func TestResolveIdentityRejectsExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	verifier := &verifierStub{err: agentsvc.ErrAgentTokenExpired}
	router := gin.New()
	router.Use(ResolveIdentity(ResolverOptions{
		ConsoleOperatorLabel: "console-operator",
		Verifier:             verifier,
	}))
	router.GET("/identity", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/identity", nil)
	req.Header.Set("Authorization", "Bearer aitask_at_tok.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := body.Code, "AGENT_TOKEN_EXPIRED"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if got, want := verifier.lastToken, "aitask_at_tok.test"; got != want {
		t.Fatalf("token = %q, want %q", got, want)
	}
}

func TestResolveIdentityInjectsAgentIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	verifier := &verifierStub{
		identity: identity.AgentIdentity{
			TokenID:         "tok_1",
			AgentID:         "agt_1",
			AgentType:       "codex",
			Role:            "worker",
			AllowedProjects: []string{"prj_1"},
		},
	}
	router := gin.New()
	router.Use(ResolveIdentity(ResolverOptions{
		ConsoleOperatorLabel: "console-operator",
		Verifier:             verifier,
	}))
	router.GET("/identity", func(c *gin.Context) {
		value := IdentityFromContext(c)
		c.JSON(http.StatusOK, gin.H{
			"senderType": string(value.SenderType),
			"agentId":    value.Agent.AgentID,
			"agentType":  value.Agent.AgentType,
			"role":       value.Agent.Role,
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/identity", nil)
	req.Header.Set("Authorization", "Bearer aitask_at_tok.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		SenderType string `json:"senderType"`
		AgentID    string `json:"agentId"`
		AgentType  string `json:"agentType"`
		Role       string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := body.SenderType, "agent"; got != want {
		t.Fatalf("senderType = %q, want %q", got, want)
	}
	if got, want := body.AgentID, "agt_1"; got != want {
		t.Fatalf("agentId = %q, want %q", got, want)
	}
}

func TestRequireProjectAccessRejectsUnauthorizedProject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	verifier := &verifierStub{
		identity: identity.AgentIdentity{
			TokenID:         "tok_1",
			AgentID:         "agt_1",
			AllowedProjects: []string{"prj_allowed"},
		},
	}
	router := gin.New()
	router.Use(ResolveIdentity(ResolverOptions{
		ConsoleOperatorLabel: "console-operator",
		Verifier:             verifier,
	}))
	router.GET("/projects/:projectId/tasks", RequireProjectAccess(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/projects/prj_denied/tasks", nil)
	req.Header.Set("Authorization", "Bearer aitask_at_tok.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := body.Code, "PROJECT_ACCESS_DENIED"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
}
