package contextsvc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/pkg/ids"
)

var (
	ErrContextRunNotFound      = errors.New("context run not found")
	ErrContextRunAccessDenied  = errors.New("context run access denied")
	ErrContextHandoffRequired  = errors.New("context handoff required")
	ErrContextBudgetExceeded   = errors.New("context budget exceeded")
	ErrContextHandoffNotFound  = errors.New("context handoff not found")
	ErrContextInvalidArguments = errors.New("context invalid arguments")
	ErrProjectArchived         = errors.New("project archived")
)

const defaultMaxContextTokens = 200_000

type MemoryWriter interface {
	Write(ctx context.Context, projectID string, input openviking.WriteInput) (openviking.WriteResult, error)
}

type HandoffPublisher interface {
	PublishHandoffCreated(ctx context.Context, projectID string, handoffID string, taskID string)
}

type Options struct {
	DB                  *sql.DB
	MemoryWriter        MemoryWriter
	HandoffPublisher    HandoffPublisher
	OpenVikingNamespace string
	Logger              *slog.Logger
	Now                 func() time.Time
}

type Service struct {
	db               *sql.DB
	memoryWriter     MemoryWriter
	handoffPublisher HandoffPublisher
	namespace        string
	logger           *slog.Logger
	now              func() time.Time
}

type Budget struct {
	MaxContextTokens    int     `json:"maxContextTokens"`
	EstimatedUsedTokens int     `json:"estimatedUsedTokens"`
	State               string  `json:"state"`
	UsageRatio          float64 `json:"usageRatio"`
}

type NextAction struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
}

type ReportInput struct {
	ProjectID            string
	RunID                string
	ReportedInputTokens  int
	ReportedOutputTokens int
	MaxContextTokens     int
	Source               string
}

type ReportOutput struct {
	Budget     Budget     `json:"budget"`
	Warnings   []string   `json:"warnings"`
	NextAction NextAction `json:"nextAction"`
}

type CreateHandoffInput struct {
	ProjectID       string
	TaskID          string
	Reason          string
	HandoffMarkdown string
}

type CreateHandoffOutput struct {
	HandoffID     string     `json:"handoffId"`
	OpenVikingURI string     `json:"openvikingUri"`
	NextAction    NextAction `json:"nextAction"`
}

type ContextRef struct {
	URI             string `json:"uri"`
	Title           string `json:"title"`
	EstimatedTokens int    `json:"estimatedTokens,omitempty"`
}

type CurrentHandoffOutput struct {
	HandoffID       string       `json:"handoffId"`
	TaskID          string       `json:"taskId"`
	Summary         string       `json:"summary"`
	HandoffMarkdown string       `json:"handoffMarkdown"`
	ContextRefs     []ContextRef `json:"contextRefs"`
}

func New(opts Options) (*Service, error) {
	if opts.DB == nil {
		return nil, errors.New("context service requires database handle")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	namespace := strings.TrimSpace(opts.OpenVikingNamespace)
	if namespace == "" {
		namespace = "aitask"
	}
	return &Service{
		db:               opts.DB,
		memoryWriter:     opts.MemoryWriter,
		handoffPublisher: opts.HandoffPublisher,
		namespace:        namespace,
		logger:           logger,
		now:              now,
	}, nil
}

func (s *Service) Report(ctx context.Context, actor identity.Identity, input ReportInput) (ReportOutput, error) {
	if !actor.IsAgent() {
		return ReportOutput{}, ErrContextRunAccessDenied
	}
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.RunID = strings.TrimSpace(input.RunID)
	input.Source = strings.TrimSpace(input.Source)
	if input.Source == "" {
		input.Source = actor.Agent.AgentType
	}

	details := map[string]string{}
	if input.ProjectID == "" {
		details["projectId"] = "cannot be empty"
	}
	if input.RunID == "" {
		details["runId"] = "cannot be empty"
	}
	if input.ReportedInputTokens < 0 {
		details["reportedInputTokens"] = "cannot be negative"
	}
	if input.ReportedOutputTokens < 0 {
		details["reportedOutputTokens"] = "cannot be negative"
	}
	if len(details) > 0 {
		return ReportOutput{}, invalidContextError(details)
	}
	if err := s.assertProjectWritable(ctx, input.ProjectID); err != nil {
		return ReportOutput{}, err
	}

	var row struct {
		ProjectID string
		AgentID   string
		MaxTokens int
		Used      int
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT project_id, agent_id, COALESCE(max_context_tokens, 0), COALESCE(estimated_used_tokens, 0)
		FROM agent_runs
		WHERE id = $1 AND status = 'active'
	`, input.RunID).Scan(&row.ProjectID, &row.AgentID, &row.MaxTokens, &row.Used)
	if errors.Is(err, sql.ErrNoRows) {
		return ReportOutput{}, ErrContextRunNotFound
	}
	if err != nil {
		return ReportOutput{}, fmt.Errorf("load run failed: %w", err)
	}
	if row.ProjectID != input.ProjectID || row.AgentID != actor.Agent.AgentID {
		return ReportOutput{}, ErrContextRunAccessDenied
	}

	maxTokens := input.MaxContextTokens
	if maxTokens <= 0 {
		maxTokens = row.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = defaultMaxContextTokens
	}
	usedTokens := input.ReportedInputTokens + input.ReportedOutputTokens
	if usedTokens <= 0 {
		usedTokens = row.Used
	}
	if usedTokens < 0 {
		usedTokens = 0
	}

	budget := buildBudget(maxTokens, usedTokens)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE agent_runs
		SET max_context_tokens = $2,
			estimated_used_tokens = $3,
			context_state = $4,
			last_heartbeat_at = NOW()
		WHERE id = $1
	`, input.RunID, budget.MaxContextTokens, budget.EstimatedUsedTokens, budget.State); err != nil {
		return ReportOutput{}, fmt.Errorf("update run context failed: %w", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"source":               input.Source,
		"reportedInputTokens":  input.ReportedInputTokens,
		"reportedOutputTokens": input.ReportedOutputTokens,
		"usageRatio":           budget.UsageRatio,
	})
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_run_context_usage (
			id, run_id, project_id, source,
			estimated_input_tokens, estimated_output_tokens,
			reported_input_tokens, reported_output_tokens,
			total_estimated_tokens, max_context_tokens, state, payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, ids.New("ctxu"), input.RunID, input.ProjectID, input.Source,
		input.ReportedInputTokens, input.ReportedOutputTokens,
		input.ReportedInputTokens, input.ReportedOutputTokens,
		budget.EstimatedUsedTokens, budget.MaxContextTokens, budget.State, payload); err != nil {
		return ReportOutput{}, fmt.Errorf("append context usage failed: %w", err)
	}

	warnings, next := actionForState(budget.State)
	return ReportOutput{
		Budget:     budget,
		Warnings:   warnings,
		NextAction: next,
	}, nil
}

func (s *Service) CreateHandoff(ctx context.Context, actor identity.Identity, input CreateHandoffInput) (CreateHandoffOutput, error) {
	if !actor.IsAgent() {
		return CreateHandoffOutput{}, ErrContextRunAccessDenied
	}
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.TaskID = strings.TrimSpace(input.TaskID)
	input.Reason = strings.TrimSpace(input.Reason)
	input.HandoffMarkdown = strings.TrimSpace(input.HandoffMarkdown)
	if input.Reason == "" {
		input.Reason = "context_limit_handoff"
	}
	details := map[string]string{}
	if input.ProjectID == "" {
		details["projectId"] = "cannot be empty"
	}
	if input.TaskID == "" {
		details["taskId"] = "cannot be empty"
	}
	if input.HandoffMarkdown == "" {
		details["handoffMarkdown"] = "cannot be empty"
	}
	if len(details) > 0 {
		return CreateHandoffOutput{}, invalidContextError(details)
	}
	if err := s.assertProjectWritable(ctx, input.ProjectID); err != nil {
		return CreateHandoffOutput{}, err
	}

	var task struct {
		SessionID string
		RunID     sql.NullString
		AgentID   sql.NullString
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT session_id, active_run_id, assignee_agent_id
		FROM tasks
		WHERE project_id = $1 AND id = $2
	`, input.ProjectID, input.TaskID).Scan(&task.SessionID, &task.RunID, &task.AgentID)
	if errors.Is(err, sql.ErrNoRows) {
		return CreateHandoffOutput{}, ErrContextInvalidArguments
	}
	if err != nil {
		return CreateHandoffOutput{}, fmt.Errorf("load task for handoff failed: %w", err)
	}
	if !task.AgentID.Valid || strings.TrimSpace(task.AgentID.String) != actor.Agent.AgentID {
		return CreateHandoffOutput{}, ErrContextRunAccessDenied
	}

	fromRunID := strings.TrimSpace(task.RunID.String)
	if fromRunID == "" {
		return CreateHandoffOutput{}, ErrContextRunNotFound
	}

	handoffID := ids.New(ids.PrefixHandoff)
	summary := summarizeMarkdown(input.HandoffMarkdown)
	refs := parseRefs(input.HandoffMarkdown)
	openVikingURI := fmt.Sprintf("viking://%s/projects/%s/memory/handoffs/%s.md", s.namespace, input.ProjectID, handoffID)

	if s.memoryWriter != nil {
		if result, err := s.memoryWriter.Write(ctx, input.ProjectID, openviking.WriteInput{
			Target:        "handoff",
			Title:         handoffID,
			Content:       input.HandoffMarkdown,
			RelatedTaskID: input.TaskID,
			AutoSync:      true,
		}); err != nil {
			s.logger.Warn("handoff write to memory failed", "projectId", input.ProjectID, "handoffId", handoffID, "error", err)
		} else if strings.TrimSpace(result.URI) != "" {
			openVikingURI = strings.TrimSpace(result.URI)
		}
	}

	localStateRaw, _ := json.Marshal(map[string]any{"handoffMarkdown": input.HandoffMarkdown})
	refsRaw, _ := json.Marshal(refs)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO context_handoffs (
			id, project_id, session_id,
			from_agent_id, from_run_id,
			to_agent_id, to_agent_type,
			task_id, status, reason,
			summary, openviking_refs, local_state, openviking_uri
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'created',$9,$10,$11,$12,$13)
	`,
		handoffID, input.ProjectID, task.SessionID,
		actor.Agent.AgentID, fromRunID,
		actor.Agent.AgentID, nullableString(actor.Agent.AgentType),
		input.TaskID, input.Reason, summary,
		refsRaw, localStateRaw, openVikingURI,
	); err != nil {
		return CreateHandoffOutput{}, fmt.Errorf("insert context handoff failed: %w", err)
	}

	if s.handoffPublisher != nil {
		s.handoffPublisher.PublishHandoffCreated(ctx, input.ProjectID, handoffID, input.TaskID)
	}

	return CreateHandoffOutput{
		HandoffID:     handoffID,
		OpenVikingURI: openVikingURI,
		NextAction: NextAction{
			Type:    "run_end",
			Message: "End current run after handoff is saved",
			Command: "aitask run end --reason context_limit_handoff",
		},
	}, nil
}

func (s *Service) GetCurrentHandoff(ctx context.Context, actor identity.Identity, projectID string) (CurrentHandoffOutput, error) {
	if !actor.IsAgent() {
		return CurrentHandoffOutput{}, ErrContextRunAccessDenied
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return CurrentHandoffOutput{}, invalidContextError(map[string]string{"projectId": "cannot be empty"})
	}

	var row struct {
		HandoffID  string
		TaskID     sql.NullString
		Summary    string
		LocalState []byte
		Refs       []byte
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, summary, COALESCE(local_state, '{}'::jsonb), COALESCE(openviking_refs, '[]'::jsonb)
		FROM context_handoffs
		WHERE project_id = $1
		  AND consumed_at IS NULL
		  AND to_agent_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, projectID, actor.Agent.AgentID).Scan(&row.HandoffID, &row.TaskID, &row.Summary, &row.LocalState, &row.Refs)
	if errors.Is(err, sql.ErrNoRows) {
		return CurrentHandoffOutput{}, ErrContextHandoffNotFound
	}
	if err != nil {
		return CurrentHandoffOutput{}, fmt.Errorf("get current handoff failed: %w", err)
	}

	handoffMarkdown := ""
	var localState map[string]any
	if len(row.LocalState) > 0 {
		_ = json.Unmarshal(row.LocalState, &localState)
		if value, ok := localState["handoffMarkdown"].(string); ok {
			handoffMarkdown = value
		}
	}

	refs := make([]ContextRef, 0)
	if len(row.Refs) > 0 {
		_ = json.Unmarshal(row.Refs, &refs)
	}
	if len(refs) == 0 {
		refs = []ContextRef{}
	}

	taskID := ""
	if row.TaskID.Valid {
		taskID = strings.TrimSpace(row.TaskID.String)
	}

	return CurrentHandoffOutput{
		HandoffID:       row.HandoffID,
		TaskID:          taskID,
		Summary:         row.Summary,
		HandoffMarkdown: handoffMarkdown,
		ContextRefs:     refs,
	}, nil
}

func (s *Service) assertProjectWritable(ctx context.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return invalidContextError(map[string]string{"projectId": "cannot be empty"})
	}
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM projects WHERE id = $1`, projectID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrContextInvalidArguments
	}
	if err != nil {
		return fmt.Errorf("load project writable state failed: %w", err)
	}
	if strings.TrimSpace(status) == "archived" {
		return ErrProjectArchived
	}
	return nil
}

func (s *Service) HasPendingHandoff(ctx context.Context, actor identity.Identity, projectID string) (bool, error) {
	if !actor.IsAgent() {
		return false, nil
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return false, nil
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM context_handoffs
		WHERE project_id = $1
		  AND consumed_at IS NULL
		  AND to_agent_id = $2
	`, projectID, actor.Agent.AgentID).Scan(&count); err != nil {
		return false, fmt.Errorf("query pending handoff failed: %w", err)
	}
	return count > 0, nil
}

func (s *Service) FindActiveRunBudget(ctx context.Context, actor identity.Identity, projectID string) (string, Budget, error) {
	if !actor.IsAgent() {
		return "", Budget{}, nil
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", Budget{}, nil
	}

	var runID string
	var maxTokens int
	var usedTokens int
	var state sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT ar.id, COALESCE(ar.max_context_tokens, 0), COALESCE(ar.estimated_used_tokens, 0), COALESCE(ar.context_state, 'normal')
		FROM tasks t
		JOIN agent_runs ar ON ar.id = t.active_run_id
		WHERE t.project_id = $1
		  AND t.assignee_agent_id = $2
		  AND t.status = 'running'
		  AND ar.status = 'active'
		ORDER BY t.updated_at DESC
		LIMIT 1
	`, projectID, actor.Agent.AgentID).Scan(&runID, &maxTokens, &usedTokens, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", Budget{MaxContextTokens: defaultMaxContextTokens, EstimatedUsedTokens: 0, State: "normal", UsageRatio: 0}, nil
	}
	if err != nil {
		return "", Budget{}, fmt.Errorf("load active run budget failed: %w", err)
	}

	budget := buildBudget(maxTokens, usedTokens)
	if state.Valid && strings.TrimSpace(state.String) != "" {
		budget.State = strings.TrimSpace(state.String)
	}
	return runID, budget, nil
}

func (s *Service) GetRunState(ctx context.Context, runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", ErrContextRunNotFound
	}
	var state sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT context_state FROM agent_runs WHERE id = $1`, runID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrContextRunNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load run state failed: %w", err)
	}
	if !state.Valid || strings.TrimSpace(state.String) == "" {
		return "normal", nil
	}
	return strings.TrimSpace(state.String), nil
}

func buildBudget(maxTokens int, usedTokens int) Budget {
	if maxTokens <= 0 {
		maxTokens = defaultMaxContextTokens
	}
	if usedTokens < 0 {
		usedTokens = 0
	}
	usageRatio := float64(usedTokens) / float64(maxTokens)
	state := stateFromRatio(usageRatio)
	return Budget{MaxContextTokens: maxTokens, EstimatedUsedTokens: usedTokens, State: state, UsageRatio: usageRatio}
}

func stateFromRatio(ratio float64) string {
	switch {
	case ratio >= 0.95:
		return "handoff_only"
	case ratio >= 0.90:
		return "handoff_required"
	case ratio >= 0.85:
		return "restricted"
	case ratio >= 0.70:
		return "compressed"
	default:
		return "normal"
	}
}

func actionForState(state string) ([]string, NextAction) {
	switch strings.TrimSpace(state) {
	case "compressed":
		return []string{"Context usage is increasing; prefer summaries and refs."}, NextAction{Type: "continue", Message: "Prefer compact context reads."}
	case "restricted":
		return []string{"Context usage is high. Prepare handoff if work remains complex."}, NextAction{Type: "prepare_handoff", Message: "Prepare handoff if continuing complex work", Command: "aitask context handoff prepare"}
	case "handoff_required":
		return []string{"Context limit approaching hard boundary. Handoff is required."}, NextAction{Type: "handoff_required", Message: "Create handoff before continuing", Command: "aitask context handoff submit --from .aitask/handoff.md"}
	case "handoff_only":
		return []string{"Context budget exceeded 95%. Only checkpoint/handoff/heartbeat/run end are allowed."}, NextAction{Type: "handoff_only", Message: "End run after creating handoff", Command: "aitask run end --reason context_limit_handoff"}
	default:
		return []string{}, NextAction{Type: "continue", Message: "Continue task execution."}
	}
}

func summarizeMarkdown(markdown string) string {
	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimLeft(trimmed, "#- ")
		if trimmed != "" {
			if len(trimmed) > 200 {
				return trimmed[:200]
			}
			return trimmed
		}
	}
	return "Handoff summary"
}

var refPattern = regexp.MustCompile(`(viking://[^\s)]+|ov://[^\s)]+)`)

func parseRefs(markdown string) []ContextRef {
	matches := refPattern.FindAllString(markdown, -1)
	if len(matches) == 0 {
		return []ContextRef{}
	}
	set := map[string]struct{}{}
	items := make([]ContextRef, 0, len(matches))
	for _, match := range matches {
		uri := strings.TrimSpace(match)
		if uri == "" {
			continue
		}
		if _, ok := set[uri]; ok {
			continue
		}
		set[uri] = struct{}{}
		items = append(items, ContextRef{URI: uri, Title: "Context Ref", EstimatedTokens: 300})
	}
	return items
}

type invalidError struct {
	details map[string]string
}

func (e *invalidError) Error() string {
	return ErrContextInvalidArguments.Error()
}

func (e *invalidError) Unwrap() error {
	return ErrContextInvalidArguments
}

func (e *invalidError) Details() map[string]string {
	out := make(map[string]string, len(e.details))
	for k, v := range e.details {
		out[k] = v
	}
	return out
}

func invalidContextError(details map[string]string) error {
	return &invalidError{details: details}
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func IsInvalidInput(err error) (map[string]string, bool) {
	var invalid *invalidError
	if !errors.As(err, &invalid) {
		return nil, false
	}
	return invalid.Details(), true
}

func (s *Service) SyncPendingHandoffs(ctx context.Context, limit int) (int, error) {
	if s.memoryWriter == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, COALESCE(task_id, ''), COALESCE(summary, ''), COALESCE(local_state, '{}'::jsonb)
		FROM context_handoffs
		WHERE COALESCE(status, 'created') <> 'synced'
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("list pending handoffs failed: %w", err)
	}
	defer rows.Close()

	type handoffItem struct {
		HandoffID  string
		ProjectID  string
		TaskID     string
		Summary    string
		LocalState []byte
	}
	items := make([]handoffItem, 0)
	for rows.Next() {
		var item handoffItem
		if err := rows.Scan(&item.HandoffID, &item.ProjectID, &item.TaskID, &item.Summary, &item.LocalState); err != nil {
			return 0, fmt.Errorf("scan pending handoff failed: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate pending handoffs failed: %w", err)
	}

	synced := 0
	for _, item := range items {
		content := strings.TrimSpace(item.Summary)
		if len(item.LocalState) > 0 {
			var localState map[string]any
			if unmarshalErr := json.Unmarshal(item.LocalState, &localState); unmarshalErr == nil {
				if markdown, ok := localState["handoffMarkdown"].(string); ok && strings.TrimSpace(markdown) != "" {
					content = strings.TrimSpace(markdown)
				}
			}
		}
		if content == "" {
			content = item.Summary
		}
		if strings.TrimSpace(content) == "" {
			content = "Context handoff snapshot"
		}

		writeResult, err := s.memoryWriter.Write(ctx, item.ProjectID, openviking.WriteInput{
			Target:        "handoff",
			Title:         item.HandoffID,
			Content:       content,
			RelatedTaskID: strings.TrimSpace(item.TaskID),
			AutoSync:      true,
		})
		if err != nil {
			s.logger.Warn("worker handoff sync failed", "projectId", item.ProjectID, "handoffId", item.HandoffID, "error", err)
			continue
		}

		_, err = s.db.ExecContext(ctx, `
			UPDATE context_handoffs
			SET status = 'synced',
				openviking_uri = COALESCE(NULLIF($2, ''), openviking_uri)
			WHERE id = $1
		`, item.HandoffID, strings.TrimSpace(writeResult.URI))
		if err != nil {
			return synced, fmt.Errorf("mark handoff synced failed: %w", err)
		}
		synced++
	}
	return synced, nil
}
