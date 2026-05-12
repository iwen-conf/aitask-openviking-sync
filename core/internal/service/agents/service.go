package agents

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/pkg/ids"
)

const tokenPrefix = "aitask_at_"

var (
	allowedAgentTypes = map[string]struct{}{
		"claude-code": {},
		"codex":       {},
		"gemini":      {},
		"system":      {},
	}

	ErrAgentNotFound           = errors.New("agent not found")
	ErrAgentTypeInvalid        = errors.New("agent type invalid")
	ErrAgentTokenInvalid       = errors.New("agent token invalid")
	ErrAgentTokenExpired       = errors.New("agent token expired")
	ErrAgentNotBoundToProject  = errors.New("agent not bound to project")
	ErrAgentTokenIssueRejected = errors.New("agent token issue rejected")
	ErrProjectArchived         = errors.New("project archived")
)

type InvalidInputError struct {
	details map[string]string
}

func (e *InvalidInputError) Error() string {
	return "invalid agent input"
}

func (e *InvalidInputError) Details() map[string]string {
	if len(e.details) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(e.details))
	for k, v := range e.details {
		out[k] = v
	}
	return out
}

func newInvalidInputError(details map[string]string) error {
	copied := make(map[string]string, len(details))
	for k, v := range details {
		copied[k] = v
	}
	return &InvalidInputError{details: copied}
}

type Options struct {
	DB          *sql.DB
	TokenSecret string
	Now         func() time.Time
	Random      io.Reader
}

type Service struct {
	db          *sql.DB
	tokenSecret []byte
	now         func() time.Time
	random      io.Reader
}

type CreateAgentInput struct {
	Name         string
	AgentType    string
	Role         string
	DefaultModel string
	Skills       []string
	Models       []string
}

type AgentTokenSummary struct {
	TokenID   string
	ExpiresAt *time.Time
	RevokedAt *time.Time
	Scopes    []string
}

type Agent struct {
	AgentID       string
	Name          string
	AgentType     string
	Role          string
	Status        string
	Scopes        []string
	DefaultModel  string
	Models        []string
	Skills        []string
	BoundProjects []string
	LastSeenAt    *time.Time
	CurrentTaskID *string
	Tokens        []AgentTokenSummary
}

type IssueTokenInput struct {
	ExpiresAt *time.Time
	Scopes    []string
}

type IssueTokenOutput struct {
	TokenID    string
	AgentToken string
	ExpiresAt  *time.Time
}

type RevokeTokenOutput struct {
	TokenID   string
	RevokedAt time.Time
}

type BindInput struct {
	Role    string
	Enabled bool
}

type BindOutput struct {
	ProjectID string
	AgentID   string
	Role      string
	Enabled   bool
}

func New(opts Options) (*Service, error) {
	if opts.DB == nil {
		return nil, errors.New("agents service requires database handle")
	}
	secret := strings.TrimSpace(opts.TokenSecret)
	if secret == "" {
		return nil, errors.New("agents service requires token secret")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	random := opts.Random
	if random == nil {
		random = rand.Reader
	}
	return &Service{
		db:          opts.DB,
		tokenSecret: []byte(secret),
		now:         now,
		random:      random,
	}, nil
}

func (s *Service) Create(ctx context.Context, input CreateAgentInput) (Agent, error) {
	input = normalizeCreateInput(input)
	if details := validateCreateInput(input); len(details) > 0 {
		return Agent{}, newInvalidInputError(details)
	}

	agentID := ids.New(ids.PrefixAgent)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO agents (id, name, agent_type, role, default_model, status)
		VALUES ($1, $2, $3, $4, $5, 'active')
	`, agentID, input.Name, input.AgentType, input.Role, nullableString(input.DefaultModel)); err != nil {
		return Agent{}, fmt.Errorf("create agent failed: %w", err)
	}

	if err := s.replaceSkills(ctx, agentID, input.Skills); err != nil {
		return Agent{}, err
	}
	if err := s.replaceModels(ctx, agentID, input.Models); err != nil {
		return Agent{}, err
	}

	return Agent{
		AgentID:      agentID,
		Name:         input.Name,
		AgentType:    input.AgentType,
		Role:         input.Role,
		Status:       "active",
		DefaultModel: input.DefaultModel,
		Skills:       uniqueStrings(input.Skills),
		Models:       uniqueStrings(input.Models),
		Scopes:       []string{},
		Tokens:       []AgentTokenSummary{},
	}, nil
}

func (s *Service) List(ctx context.Context) ([]Agent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, agent_type, role, status, COALESCE(default_model, '')
		FROM agents
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list agents query failed: %w", err)
	}
	defer rows.Close()

	agents := make([]Agent, 0)
	for rows.Next() {
		var agent Agent
		if err := rows.Scan(&agent.AgentID, &agent.Name, &agent.AgentType, &agent.Role, &agent.Status, &agent.DefaultModel); err != nil {
			return nil, fmt.Errorf("scan agent row failed: %w", err)
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent rows failed: %w", err)
	}

	for i := range agents {
		agentID := agents[i].AgentID
		agents[i].Skills, err = s.loadStrings(ctx, `SELECT skill_name FROM agent_skills WHERE agent_id = $1 ORDER BY skill_name ASC`, agentID)
		if err != nil {
			return nil, err
		}
		agents[i].Models, err = s.loadStrings(ctx, `SELECT model_name FROM agent_models WHERE agent_id = $1 ORDER BY model_name ASC`, agentID)
		if err != nil {
			return nil, err
		}
		agents[i].BoundProjects, err = s.loadStrings(ctx, `SELECT project_id FROM agent_project_bindings WHERE agent_id = $1 AND enabled = TRUE ORDER BY project_id ASC`, agentID)
		if err != nil {
			return nil, err
		}
		agents[i].Tokens, err = s.loadTokenSummaries(ctx, agentID)
		if err != nil {
			return nil, err
		}
		agents[i].Scopes = aggregateActiveScopes(agents[i].Tokens, s.now())
	}

	return agents, nil
}

func (s *Service) IssueToken(ctx context.Context, agentID string, input IssueTokenInput) (IssueTokenOutput, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return IssueTokenOutput{}, newInvalidInputError(map[string]string{"agentId": "cannot be empty"})
	}
	agentType, exists, err := s.loadAgentType(ctx, agentID)
	if err != nil {
		return IssueTokenOutput{}, err
	}
	if !exists {
		return IssueTokenOutput{}, ErrAgentNotFound
	}

	now := s.now().UTC()
	var expiresAt *time.Time
	if input.ExpiresAt != nil {
		value := input.ExpiresAt.UTC()
		if !value.After(now) {
			return IssueTokenOutput{}, newInvalidInputError(map[string]string{"expiresAt": "must be in the future"})
		}
		expiresAt = &value
	}

	tokenID := ids.New("tok")
	secretRaw := make([]byte, 24)
	if _, err := io.ReadFull(s.random, secretRaw); err != nil {
		return IssueTokenOutput{}, fmt.Errorf("generate token random failed: %w", err)
	}
	secretPart := base64.RawURLEncoding.EncodeToString(secretRaw)
	plainToken := tokenPrefix + tokenID + "." + secretPart
	tokenHash := s.tokenHash(plainToken)
	scopes := uniqueStrings(input.Scopes)
	if len(scopes) == 0 {
		if template, ok := DefaultTemplateByType(agentType); ok {
			scopes = template.Scopes
		}
	}

	expiresArg := sql.NullTime{}
	if expiresAt != nil {
		expiresArg = sql.NullTime{Time: *expiresAt, Valid: true}
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_tokens (id, agent_id, token_hash, scopes, expires_at)
		VALUES ($1, $2, $3, $4::text[], $5)
	`, tokenID, agentID, tokenHash, toPostgresTextArray(scopes), expiresArg); err != nil {
		return IssueTokenOutput{}, fmt.Errorf("issue agent token failed: %w", err)
	}

	return IssueTokenOutput{TokenID: tokenID, AgentToken: plainToken, ExpiresAt: expiresAt}, nil
}

func (s *Service) RevokeToken(ctx context.Context, agentID string, tokenID string, _ string) (RevokeTokenOutput, error) {
	agentID = strings.TrimSpace(agentID)
	tokenID = strings.TrimSpace(tokenID)
	if agentID == "" || tokenID == "" {
		details := map[string]string{}
		if agentID == "" {
			details["agentId"] = "cannot be empty"
		}
		if tokenID == "" {
			details["tokenId"] = "cannot be empty"
		}
		return RevokeTokenOutput{}, newInvalidInputError(details)
	}

	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE agent_tokens
		SET revoked_at = $3
		WHERE id = $1 AND agent_id = $2 AND revoked_at IS NULL
	`, tokenID, agentID, now)
	if err != nil {
		return RevokeTokenOutput{}, fmt.Errorf("revoke token failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return RevokeTokenOutput{}, fmt.Errorf("revoke token rows affected failed: %w", err)
	}
	if affected == 0 {
		if exists, existsErr := s.tokenExists(ctx, agentID, tokenID); existsErr != nil {
			return RevokeTokenOutput{}, existsErr
		} else if !exists {
			return RevokeTokenOutput{}, ErrAgentTokenInvalid
		}
		// idempotent revoke
		var revokedAt time.Time
		if err := s.db.QueryRowContext(ctx, `SELECT revoked_at FROM agent_tokens WHERE id = $1 AND agent_id = $2`, tokenID, agentID).Scan(&revokedAt); err == nil {
			return RevokeTokenOutput{TokenID: tokenID, RevokedAt: revokedAt}, nil
		}
		return RevokeTokenOutput{}, ErrAgentTokenInvalid
	}
	return RevokeTokenOutput{TokenID: tokenID, RevokedAt: now}, nil
}

func (s *Service) BindProject(ctx context.Context, projectID string, agentID string, input BindInput) (BindOutput, error) {
	projectID = strings.TrimSpace(projectID)
	agentID = strings.TrimSpace(agentID)
	input.Role = strings.TrimSpace(input.Role)
	details := map[string]string{}
	if projectID == "" {
		details["projectId"] = "cannot be empty"
	}
	if agentID == "" {
		details["agentId"] = "cannot be empty"
	}
	if input.Role == "" {
		details["role"] = "cannot be empty"
	}
	if len(details) > 0 {
		return BindOutput{}, newInvalidInputError(details)
	}

	if exists, err := s.agentExists(ctx, agentID); err != nil {
		return BindOutput{}, err
	} else if !exists {
		return BindOutput{}, ErrAgentNotFound
	}
	if archived, err := s.projectArchived(ctx, projectID); err != nil {
		return BindOutput{}, err
	} else if archived {
		return BindOutput{}, ErrProjectArchived
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_project_bindings (agent_id, project_id, role, enabled)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (agent_id, project_id)
		DO UPDATE SET role = EXCLUDED.role, enabled = EXCLUDED.enabled
	`, agentID, projectID, input.Role, input.Enabled); err != nil {
		return BindOutput{}, fmt.Errorf("bind agent project failed: %w", err)
	}

	return BindOutput{ProjectID: projectID, AgentID: agentID, Role: input.Role, Enabled: input.Enabled}, nil
}

func (s *Service) projectArchived(ctx context.Context, projectID string) (bool, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM projects WHERE id = $1`, projectID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load project status failed: %w", err)
	}
	return strings.TrimSpace(status) == "archived", nil
}

func (s *Service) loadAgentType(ctx context.Context, agentID string) (string, bool, error) {
	var agentType string
	err := s.db.QueryRowContext(ctx, `SELECT agent_type FROM agents WHERE id = $1`, agentID).Scan(&agentType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load agent type failed: %w", err)
	}
	return strings.TrimSpace(agentType), true, nil
}

func (s *Service) VerifyToken(ctx context.Context, plainToken string) (identity.AgentIdentity, error) {
	plainToken = strings.TrimSpace(plainToken)
	if plainToken == "" || !strings.HasPrefix(plainToken, tokenPrefix) {
		return identity.AgentIdentity{}, ErrAgentTokenInvalid
	}

	tokenHash := s.tokenHash(plainToken)
	var row struct {
		TokenID   string
		AgentID   string
		ScopesRaw string
		ExpiresAt sql.NullTime
		RevokedAt sql.NullTime
		AgentType string
		Role      string
		Status    string
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.agent_id, COALESCE(array_to_json(t.scopes)::text, '[]'), t.expires_at, t.revoked_at, a.agent_type, a.role, a.status
		FROM agent_tokens t
		JOIN agents a ON a.id = t.agent_id
		WHERE t.token_hash = $1
	`, tokenHash).Scan(
		&row.TokenID,
		&row.AgentID,
		&row.ScopesRaw,
		&row.ExpiresAt,
		&row.RevokedAt,
		&row.AgentType,
		&row.Role,
		&row.Status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.AgentIdentity{}, ErrAgentTokenInvalid
	}
	if err != nil {
		return identity.AgentIdentity{}, fmt.Errorf("verify token query failed: %w", err)
	}

	if row.RevokedAt.Valid {
		return identity.AgentIdentity{}, ErrAgentTokenInvalid
	}
	if row.ExpiresAt.Valid && !row.ExpiresAt.Time.After(s.now()) {
		return identity.AgentIdentity{}, ErrAgentTokenExpired
	}
	if strings.EqualFold(strings.TrimSpace(row.Status), "inactive") {
		return identity.AgentIdentity{}, ErrAgentTokenInvalid
	}

	allowedProjects, err := s.loadStrings(ctx, `SELECT project_id FROM agent_project_bindings WHERE agent_id = $1 AND enabled = TRUE ORDER BY project_id ASC`, row.AgentID)
	if err != nil {
		return identity.AgentIdentity{}, err
	}

	scopes := parseJSONArray(row.ScopesRaw)

	return identity.AgentIdentity{
		TokenID:         row.TokenID,
		AgentID:         row.AgentID,
		AgentType:       row.AgentType,
		Role:            row.Role,
		Scopes:          uniqueStrings(scopes),
		AllowedProjects: uniqueStrings(allowedProjects),
	}, nil
}

func (s *Service) IsAgentBoundToProject(ctx context.Context, agentID string, projectID string) (bool, error) {
	agentID = strings.TrimSpace(agentID)
	projectID = strings.TrimSpace(projectID)
	if agentID == "" || projectID == "" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM agent_project_bindings
			WHERE agent_id = $1 AND project_id = $2 AND enabled = TRUE
		)
	`, agentID, projectID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check agent binding failed: %w", err)
	}
	return exists, nil
}

func (s *Service) agentExists(ctx context.Context, agentID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`, agentID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check agent exists failed: %w", err)
	}
	return exists, nil
}

func (s *Service) tokenExists(ctx context.Context, agentID string, tokenID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_tokens WHERE id = $1 AND agent_id = $2)`, tokenID, agentID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check token exists failed: %w", err)
	}
	return exists, nil
}

func (s *Service) replaceSkills(ctx context.Context, agentID string, skills []string) error {
	skills = uniqueStrings(skills)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM agent_skills WHERE agent_id = $1`, agentID); err != nil {
		return fmt.Errorf("replace skills cleanup failed: %w", err)
	}
	for _, skill := range skills {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO agent_skills (agent_id, skill_name) VALUES ($1, $2)`, agentID, skill); err != nil {
			return fmt.Errorf("insert skill failed: %w", err)
		}
	}
	return nil
}

func (s *Service) replaceModels(ctx context.Context, agentID string, models []string) error {
	models = uniqueStrings(models)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM agent_models WHERE agent_id = $1`, agentID); err != nil {
		return fmt.Errorf("replace models cleanup failed: %w", err)
	}
	for _, model := range models {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO agent_models (agent_id, model_name) VALUES ($1, $2)`, agentID, model); err != nil {
			return fmt.Errorf("insert model failed: %w", err)
		}
	}
	return nil
}

func (s *Service) loadStrings(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load strings query failed: %w", err)
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("load strings scan failed: %w", err)
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load strings rows failed: %w", err)
	}
	return items, nil
}

func (s *Service) loadTokenSummaries(ctx context.Context, agentID string) ([]AgentTokenSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, expires_at, revoked_at, COALESCE(array_to_json(scopes)::text, '[]')
		FROM agent_tokens
		WHERE agent_id = $1
		ORDER BY created_at DESC
	`, agentID)
	if err != nil {
		return nil, fmt.Errorf("load token summaries query failed: %w", err)
	}
	defer rows.Close()

	items := make([]AgentTokenSummary, 0)
	for rows.Next() {
		var item AgentTokenSummary
		var expiresAt sql.NullTime
		var revokedAt sql.NullTime
		var scopesRaw string
		if err := rows.Scan(&item.TokenID, &expiresAt, &revokedAt, &scopesRaw); err != nil {
			return nil, fmt.Errorf("load token summaries scan failed: %w", err)
		}
		if expiresAt.Valid {
			value := expiresAt.Time
			item.ExpiresAt = &value
		}
		if revokedAt.Valid {
			value := revokedAt.Time
			item.RevokedAt = &value
		}
		item.Scopes = uniqueStrings(parseJSONArray(scopesRaw))
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load token summaries rows failed: %w", err)
	}
	return items, nil
}

func (s *Service) tokenHash(token string) string {
	h := hmac.New(sha256.New, s.tokenSecret)
	_, _ = h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

func normalizeCreateInput(input CreateAgentInput) CreateAgentInput {
	input.Name = strings.TrimSpace(input.Name)
	input.AgentType = strings.TrimSpace(input.AgentType)
	input.Role = strings.TrimSpace(input.Role)
	input.DefaultModel = strings.TrimSpace(input.DefaultModel)
	input.Skills = uniqueStrings(input.Skills)
	input.Models = uniqueStrings(input.Models)
	return input
}

func validateCreateInput(input CreateAgentInput) map[string]string {
	details := map[string]string{}
	if n := utf8.RuneCountInString(input.Name); n < 2 || n > 80 {
		details["name"] = "must be between 2 and 80 characters"
	}
	if _, ok := allowedAgentTypes[input.AgentType]; !ok {
		details["agentType"] = "must be one of: claude-code, codex, gemini, system"
	}
	if strings.TrimSpace(input.Role) == "" {
		details["role"] = "cannot be empty"
	}
	return details
}

func aggregateActiveScopes(tokens []AgentTokenSummary, now time.Time) []string {
	set := map[string]struct{}{}
	for _, token := range tokens {
		if token.RevokedAt != nil {
			continue
		}
		if token.ExpiresAt != nil && !token.ExpiresAt.After(now) {
			continue
		}
		for _, scope := range token.Scopes {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				continue
			}
			set[scope] = struct{}{}
		}
	}
	if len(set) == 0 {
		return []string{}
	}
	items := make([]string, 0, len(set))
	for scope := range set {
		items = append(items, scope)
	}
	sort.Strings(items)
	return items
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	set := map[string]struct{}{}
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	if len(set) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return value
}

func parseJSONArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}
	}
	return items
}

func toPostgresTextArray(items []string) string {
	items = uniqueStrings(items)
	if len(items) == 0 {
		return "{}"
	}
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		escaped := strings.ReplaceAll(item, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		quoted = append(quoted, `"`+escaped+`"`)
	}
	return "{" + strings.Join(quoted, ",") + "}"
}
