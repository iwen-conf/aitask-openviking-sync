package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/pkg/ids"
)

var (
	ErrTaskNotFound                  = errors.New("task not found")
	ErrTaskNotEligibleForAgent       = errors.New("task not eligible for agent")
	ErrTaskAlreadyDelegated          = errors.New("task already delegated")
	ErrTaskNotAssignedToCurrentAgent = errors.New("task not assigned to current agent")
	ErrAgentNotBoundToProject        = errors.New("agent not bound to project")
	ErrHandoffNotFound               = errors.New("handoff not found")
	ErrHandoffAlreadyConsumed        = errors.New("handoff already consumed")
	ErrProjectAccessDenied           = errors.New("project access denied")
	ErrProjectArchived               = errors.New("project archived")
	ErrReviewScopeRequired           = errors.New("task review scope required")
	ErrContextHandoffRequired        = errors.New("context handoff required")
)

const heartbeatTimeoutBlockReason = "active_run_heartbeat_timeout"

type InvalidInputError struct {
	details map[string]string
}

func (e *InvalidInputError) Error() string {
	return "invalid task input"
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
	copyMap := make(map[string]string, len(details))
	for k, v := range details {
		copyMap[k] = v
	}
	return &InvalidInputError{details: copyMap}
}

type MemoryWriter interface {
	Write(ctx context.Context, projectID string, input openviking.WriteInput) (openviking.WriteResult, error)
}

type EventSink interface {
	PublishTaskEvent(
		ctx context.Context,
		eventID string,
		projectID string,
		taskID string,
		eventType string,
		fromStatus string,
		toStatus string,
		actor identity.Identity,
		payload map[string]any,
		createdAt time.Time,
	)
}

type Options struct {
	DB                   *sql.DB
	ConsoleOperatorLabel string
	MemoryWriter         MemoryWriter
	EventSink            EventSink
	Logger               *slog.Logger
}

type Service struct {
	db                *sql.DB
	consoleOperator   string
	memoryWriter      MemoryWriter
	eventSink         EventSink
	logger            *slog.Logger
	reviewerSkill     string
	reviewerAgentType string
}

type TaskRecord struct {
	TaskID                   string
	ProjectID                string
	Title                    string
	Description              string
	Goal                     *string
	Inputs                   *string
	Constraints              *string
	Status                   Status
	ParentTaskID             *string
	Dependencies             []string
	AssigneeAgentID          *string
	AssigneeAgentType        *string
	DelegatedByType          string
	DelegatedByOperatorLabel *string
	DelegatedByAgentID       *string
	DelegatedAt              *time.Time
	ActiveRunID              *string
	LastHeartbeatAt          *time.Time
	RequiredSkills           []string
	RequiredModel            *string
	OutputContract           *string
	Priority                 int
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type TaskFilters struct {
	Status            string
	AssigneeAgentID   string
	AssigneeAgentType string
	Skill             string
	Q                 string
}

type CreateTaskInput struct {
	Title               string
	Description         string
	Goal                string
	Inputs              string
	Constraints         string
	ParentTaskID        string
	Dependencies        []string
	DelegateToAgentID   string
	DelegateToAgentType string
	RequiredSkills      []string
	RequiredModel       string
	Priority            int
	OutputContract      string
}

type UpdateTaskInput struct {
	Title          *string
	Priority       *int
	RequiredSkills []string
}

type DelegateTaskInput struct {
	AgentID   string
	AgentType string
	Reason    string
}

type CancelTaskInput struct {
	Reason string
}

type StartTaskInput struct {
	RunID string
}

type HeartbeatTaskInput struct {
	RunID      string
	Checkpoint string
}

type SubmitArtifactInput struct {
	ArtifactType string
	URI          string
	Name         string
	Content      string
	Metadata     map[string]any
}

type SubmitTaskInput struct {
	RunID          string
	ResultMarkdown string
	Artifacts      []SubmitArtifactInput
}

type ReviewTaskInput struct {
	Approve bool
	Reason  string
}

type FailTaskInput struct {
	RunID  string
	Reason string
	Retry  bool
}

type ResumeTaskInput struct {
	HandoffID string
	RunID     string
}

type ArtifactSummary struct {
	ArtifactID   string
	TaskID       *string
	ArtifactType string
	Name         string
	Path         string
	Content      string
	Metadata     map[string]any
	CreatedAt    time.Time
}

type TaskEvent struct {
	EventID            string
	ProjectID          string
	SessionID          string
	TaskID             *string
	EventType          string
	FromStatus         *string
	ToStatus           *string
	ActorType          string
	ActorOperatorLabel *string
	ActorAgentID       *string
	ActorRunID         *string
	Payload            map[string]any
	CreatedAt          time.Time
}

func New(opts Options) (*Service, error) {
	if opts.DB == nil {
		return nil, errors.New("tasks service requires database handle")
	}
	operator := strings.TrimSpace(opts.ConsoleOperatorLabel)
	if operator == "" {
		operator = "local-operator"
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		db:                opts.DB,
		consoleOperator:   operator,
		memoryWriter:      opts.MemoryWriter,
		eventSink:         opts.EventSink,
		logger:            logger,
		reviewerSkill:     "code-review",
		reviewerAgentType: "claude-code",
	}, nil
}

func (s *Service) Create(ctx context.Context, actor identity.Identity, projectID string, input CreateTaskInput) (TaskRecord, error) {
	projectID = strings.TrimSpace(projectID)
	input = normalizeCreateInput(input)
	if details := validateCreateInput(projectID, input); len(details) > 0 {
		return TaskRecord{}, newInvalidInputError(details)
	}
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return TaskRecord{}, err
	}

	sessionID, err := s.activeSessionID(ctx, projectID)
	if err != nil {
		return TaskRecord{}, err
	}

	taskID := ids.New(ids.PrefixTask)
	status := StatusPlanned
	var assigneeID any
	var assigneeType any
	var delegatedByType any
	var delegatedByAgentID any
	var delegatedByOperator any
	var delegatedAt any

	if input.DelegateToAgentID != "" {
		resolvedType, err := s.validateAssignee(ctx, projectID, input.DelegateToAgentID, input.DelegateToAgentType, input.RequiredSkills, input.RequiredModel)
		if err != nil {
			return TaskRecord{}, err
		}
		status = StatusDelegated
		assigneeID = input.DelegateToAgentID
		assigneeType = resolvedType
		delegatedByType, delegatedByAgentID, delegatedByOperator = delegatedByFromActor(actor, s.consoleOperator)
		delegatedAt = time.Now().UTC()
	}

	createdByType, createdByAgentID, createdByOperator := createdByFromActor(actor, s.consoleOperator)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("begin create task tx failed: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tasks (
			id, project_id, session_id, parent_task_id,
			title, description, goal, input_context, constraints_text, status,
			assignee_agent_id, assignee_agent_type,
			required_model, priority, delegated_by_type, delegated_by_agent_id, delegated_by_operator_label, delegated_at,
			output_contract,
			created_by_type, created_by_operator_label, created_by_agent_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
	`,
		taskID,
		projectID,
		sessionID,
		nullableString(input.ParentTaskID),
		input.Title,
		nullableString(input.Description),
		nullableString(input.Goal),
		nullableString(input.Inputs),
		nullableString(input.Constraints),
		string(status),
		assigneeID,
		assigneeType,
		nullableString(input.RequiredModel),
		input.Priority,
		delegatedByType,
		delegatedByAgentID,
		delegatedByOperator,
		delegatedAt,
		nullableString(input.OutputContract),
		createdByType,
		createdByOperator,
		createdByAgentID,
	); err != nil {
		return TaskRecord{}, fmt.Errorf("insert task failed: %w", err)
	}

	if err := s.replaceTaskSkillsTx(ctx, tx, taskID, input.RequiredSkills); err != nil {
		return TaskRecord{}, err
	}
	if err := s.replaceTaskDependenciesTx(ctx, tx, projectID, taskID, input.Dependencies); err != nil {
		return TaskRecord{}, err
	}

	if status == StatusDelegated {
		if err := s.insertDelegationTx(ctx, tx, taskID, input.DelegateToAgentID, actor, input.ReasonOrDefault()); err != nil {
			return TaskRecord{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return TaskRecord{}, fmt.Errorf("commit create task tx failed: %w", err)
	}
	committed = true

	s.recordEventBestEffort(ctx, projectID, sessionID, taskID, "task.created", "", string(status), actor, map[string]any{"title": input.Title})
	if status == StatusDelegated {
		s.recordEventBestEffort(ctx, projectID, sessionID, taskID, "task.delegated", "planned", "delegated", actor, map[string]any{"assigneeAgentId": input.DelegateToAgentID})
	}

	return s.Get(ctx, projectID, taskID)
}

func (s *Service) List(ctx context.Context, projectID string, filters TaskFilters) ([]TaskRecord, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}

	query := `
		SELECT
			id,
			project_id,
			title,
			description,
			goal,
			input_context,
			constraints_text,
			status,
			parent_task_id,
			assignee_agent_id,
			assignee_agent_type,
			delegated_by_type,
			delegated_by_operator_label,
			delegated_by_agent_id,
			delegated_at,
			active_run_id,
			last_heartbeat_at,
			required_model,
			output_contract,
			priority,
			created_at,
			updated_at
		FROM tasks
		WHERE project_id = $1
	`
	args := []any{projectID}
	argIdx := 2
	if status := strings.TrimSpace(filters.Status); status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if assigneeID := strings.TrimSpace(filters.AssigneeAgentID); assigneeID != "" {
		query += fmt.Sprintf(" AND assignee_agent_id = $%d", argIdx)
		args = append(args, assigneeID)
		argIdx++
	}
	if assigneeType := strings.TrimSpace(filters.AssigneeAgentType); assigneeType != "" {
		query += fmt.Sprintf(" AND assignee_agent_type = $%d", argIdx)
		args = append(args, assigneeType)
		argIdx++
	}
	if skill := strings.TrimSpace(filters.Skill); skill != "" {
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM task_required_skills trs WHERE trs.task_id = tasks.id AND trs.skill_name = $%d)", argIdx)
		args = append(args, skill)
		argIdx++
	}
	if q := strings.TrimSpace(filters.Q); q != "" {
		query += fmt.Sprintf(" AND (title ILIKE $%d OR description ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+q+"%")
		argIdx++
	}
	query += ` ORDER BY created_at DESC, id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks query failed: %w", err)
	}
	defer rows.Close()

	items := make([]TaskRecord, 0)
	for rows.Next() {
		task, scanErr := scanTaskRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks rows failed: %w", err)
	}

	for i := range items {
		items[i].Dependencies, err = s.loadDependencies(ctx, items[i].TaskID)
		if err != nil {
			return nil, err
		}
		items[i].RequiredSkills, err = s.loadSkills(ctx, items[i].TaskID)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, projectID string, taskID string) (TaskRecord, error) {
	projectID = strings.TrimSpace(projectID)
	taskID = strings.TrimSpace(taskID)
	if projectID == "" || taskID == "" {
		details := map[string]string{}
		if projectID == "" {
			details["projectId"] = "cannot be empty"
		}
		if taskID == "" {
			details["taskId"] = "cannot be empty"
		}
		return TaskRecord{}, newInvalidInputError(details)
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT
			id,
			project_id,
			title,
			description,
			goal,
			input_context,
			constraints_text,
			status,
			parent_task_id,
			assignee_agent_id,
			assignee_agent_type,
			delegated_by_type,
			delegated_by_operator_label,
			delegated_by_agent_id,
			delegated_at,
			active_run_id,
			last_heartbeat_at,
			required_model,
			output_contract,
			priority,
			created_at,
			updated_at
		FROM tasks
		WHERE project_id = $1 AND id = $2
	`, projectID, taskID)

	task, err := scanTaskRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskRecord{}, ErrTaskNotFound
	}
	if err != nil {
		return TaskRecord{}, err
	}

	task.Dependencies, err = s.loadDependencies(ctx, task.TaskID)
	if err != nil {
		return TaskRecord{}, err
	}
	task.RequiredSkills, err = s.loadSkills(ctx, task.TaskID)
	if err != nil {
		return TaskRecord{}, err
	}
	return task, nil
}

func (s *Service) Update(ctx context.Context, actor identity.Identity, projectID string, taskID string, input UpdateTaskInput) (TaskRecord, error) {
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return TaskRecord{}, err
	}
	task, err := s.Get(ctx, projectID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	if !canEditTask(task.Status) {
		return TaskRecord{}, ErrTaskStatusInvalid
	}

	fields := []string{}
	args := []any{}
	idx := 1
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if n := utf8.RuneCountInString(title); n < 2 || n > 180 {
			return TaskRecord{}, newInvalidInputError(map[string]string{"title": "must be between 2 and 180 characters"})
		}
		fields = append(fields, fmt.Sprintf("title = $%d", idx))
		args = append(args, title)
		idx++
	}
	if input.Priority != nil {
		fields = append(fields, fmt.Sprintf("priority = $%d", idx))
		args = append(args, *input.Priority)
		idx++
	}
	if len(fields) == 0 && len(input.RequiredSkills) == 0 {
		return task, nil
	}
	fields = append(fields, "updated_at = NOW()")
	args = append(args, taskID, projectID)
	query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = $%d AND project_id = $%d", strings.Join(fields, ", "), idx, idx+1)
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return TaskRecord{}, fmt.Errorf("update task failed: %w", err)
	}
	if len(input.RequiredSkills) > 0 {
		if err := s.replaceTaskSkills(ctx, taskID, input.RequiredSkills); err != nil {
			return TaskRecord{}, err
		}
	}

	s.recordEventBestEffort(ctx, projectID, "", taskID, "task.updated", string(task.Status), string(task.Status), actor, map[string]any{"titleUpdated": input.Title != nil, "priorityUpdated": input.Priority != nil})

	return s.Get(ctx, projectID, taskID)
}

func (s *Service) Delegate(ctx context.Context, actor identity.Identity, projectID string, taskID string, input DelegateTaskInput) (TaskRecord, error) {
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.AgentType = strings.TrimSpace(input.AgentType)
	if input.AgentID == "" {
		return TaskRecord{}, newInvalidInputError(map[string]string{"agentId": "cannot be empty"})
	}
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return TaskRecord{}, err
	}

	task, err := s.Get(ctx, projectID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	if task.Status == StatusRunning || task.Status == StatusDone || task.Status == StatusCancelled {
		return TaskRecord{}, ErrTaskStatusInvalid
	}

	resolvedType, err := s.validateAssignee(ctx, projectID, input.AgentID, "", task.RequiredSkills, valueOrEmpty(task.RequiredModel))
	if err != nil {
		return TaskRecord{}, err
	}

	fromStatus := string(task.Status)
	toStatus := string(StatusDelegated)
	if err := ensureTransitionAllowed(task.Status, StatusDelegated); err != nil {
		return TaskRecord{}, err
	}

	delegatedByType, delegatedByAgentID, delegatedByOperator := delegatedByFromActor(actor, s.consoleOperator)
	at := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'delegated',
			assignee_agent_id = $3,
			assignee_agent_type = $4,
			delegated_by_type = $5,
			delegated_by_agent_id = $6,
			delegated_by_operator_label = $7,
			delegated_at = $8,
			active_run_id = NULL,
			started_at = NULL,
			updated_at = NOW()
		WHERE project_id = $1 AND id = $2
	`, projectID, taskID, input.AgentID, resolvedType, delegatedByType, delegatedByAgentID, delegatedByOperator, at); err != nil {
		return TaskRecord{}, fmt.Errorf("delegate task update failed: %w", err)
	}

	if err := s.insertDelegation(ctx, taskID, input.AgentID, actor, input.reasonOrDefault()); err != nil {
		return TaskRecord{}, err
	}
	s.recordEventBestEffort(ctx, projectID, "", taskID, "task.delegated", fromStatus, toStatus, actor, map[string]any{"assigneeAgentId": input.AgentID, "reason": input.reasonOrDefault()})
	return s.Get(ctx, projectID, taskID)
}

func (s *Service) Cancel(ctx context.Context, actor identity.Identity, projectID string, taskID string, input CancelTaskInput) (TaskRecord, error) {
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return TaskRecord{}, err
	}
	task, err := s.Get(ctx, projectID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	if task.Status == StatusDone || task.Status == StatusCancelled {
		return TaskRecord{}, ErrTaskStatusInvalid
	}
	if err := ensureTransitionAllowed(task.Status, StatusCancelled); err != nil {
		return TaskRecord{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'cancelled',
			error = $3,
			updated_at = NOW()
		WHERE project_id = $1 AND id = $2
	`, projectID, taskID, nullableString(strings.TrimSpace(input.Reason))); err != nil {
		return TaskRecord{}, fmt.Errorf("cancel task failed: %w", err)
	}
	s.recordEventBestEffort(ctx, projectID, "", taskID, "task.cancelled", string(task.Status), string(StatusCancelled), actor, map[string]any{"reason": strings.TrimSpace(input.Reason)})
	return s.Get(ctx, projectID, taskID)
}

func (s *Service) Start(ctx context.Context, actor identity.Identity, projectID string, taskID string, input StartTaskInput) (TaskRecord, error) {
	if !actor.IsAgent() {
		return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		return TaskRecord{}, newInvalidInputError(map[string]string{"runId": "cannot be empty"})
	}
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return TaskRecord{}, err
	}
	if state, err := s.runContextState(ctx, runID); err == nil && state == "handoff_only" {
		return TaskRecord{}, ErrContextHandoffRequired
	}

	task, err := s.Get(ctx, projectID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	if task.AssigneeAgentID == nil || *task.AssigneeAgentID != actor.Agent.AgentID {
		return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
	}
	if task.Status == StatusRunning {
		if task.ActiveRunID != nil && *task.ActiveRunID == runID {
			return task, nil
		}
		return TaskRecord{}, ErrTaskStatusInvalid
	}
	if task.Status != StatusDelegated {
		return TaskRecord{}, ErrTaskStatusInvalid
	}
	if depsDone, err := s.dependenciesDone(ctx, task.TaskID); err != nil {
		return TaskRecord{}, err
	} else if !depsDone {
		return TaskRecord{}, ErrTaskDependencyNotDone
	}

	if err := s.ensureRunExists(ctx, runID, actor.Agent.AgentID, projectID, task); err != nil {
		return TaskRecord{}, err
	}
	if err := s.enforceRunContextGate(ctx, runID, "task.start"); err != nil {
		return TaskRecord{}, err
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'running',
			active_run_id = $3,
			started_at = NOW(),
			last_heartbeat_at = NOW(),
			updated_at = NOW()
		WHERE project_id = $1 AND id = $2 AND assignee_agent_id = $4
	`, projectID, taskID, runID, actor.Agent.AgentID); err != nil {
		return TaskRecord{}, fmt.Errorf("start task failed: %w", err)
	}

	s.recordEventBestEffort(ctx, projectID, "", taskID, "task.started", string(task.Status), string(StatusRunning), actor, map[string]any{"runId": runID})
	return s.Get(ctx, projectID, taskID)
}

func (s *Service) Heartbeat(ctx context.Context, actor identity.Identity, projectID string, taskID string, input HeartbeatTaskInput) (TaskRecord, error) {
	if !actor.IsAgent() {
		return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		return TaskRecord{}, newInvalidInputError(map[string]string{"runId": "cannot be empty"})
	}
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return TaskRecord{}, err
	}
	if state, err := s.runContextState(ctx, runID); err == nil && state == "handoff_only" {
		return TaskRecord{}, ErrContextHandoffRequired
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("begin task heartbeat tx failed: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET last_heartbeat_at = NOW(),
			updated_at = NOW()
		WHERE project_id = $1 AND id = $2 AND assignee_agent_id = $3 AND active_run_id = $4 AND status = 'running'
	`, projectID, taskID, actor.Agent.AgentID, runID)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("task heartbeat failed: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		task, getErr := s.Get(ctx, projectID, taskID)
		if getErr != nil {
			return TaskRecord{}, getErr
		}
		if task.AssigneeAgentID == nil || *task.AssigneeAgentID != actor.Agent.AgentID {
			return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
		}
		if task.ActiveRunID == nil || *task.ActiveRunID != runID {
			return TaskRecord{}, ErrTaskActiveRunMismatch
		}
		return TaskRecord{}, ErrTaskStatusInvalid
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE agent_runs
		SET last_heartbeat_at = NOW()
		WHERE id = $1 AND agent_id = $2 AND project_id = $3 AND status <> 'ended'
	`, runID, actor.Agent.AgentID, projectID); err != nil {
		return TaskRecord{}, fmt.Errorf("run heartbeat failed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return TaskRecord{}, fmt.Errorf("commit task heartbeat tx failed: %w", err)
	}
	committed = true

	if checkpoint := strings.TrimSpace(input.Checkpoint); checkpoint != "" && s.memoryWriter != nil {
		_, writeErr := s.memoryWriter.Write(ctx, projectID, openviking.WriteInput{
			Target:   "note",
			Title:    fmt.Sprintf("task-%s-heartbeat", taskID),
			Content:  checkpoint,
			AutoSync: true,
		})
		if writeErr != nil {
			s.logger.Warn("task checkpoint write failed", "taskId", taskID, "projectId", projectID, "error", writeErr)
		}
	}

	return s.Get(ctx, projectID, taskID)
}

func (s *Service) Submit(ctx context.Context, actor identity.Identity, projectID string, taskID string, input SubmitTaskInput) (TaskRecord, error) {
	if !actor.IsAgent() {
		return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		return TaskRecord{}, newInvalidInputError(map[string]string{"runId": "cannot be empty"})
	}
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return TaskRecord{}, err
	}

	task, err := s.Get(ctx, projectID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	if task.Status != StatusRunning {
		return TaskRecord{}, ErrTaskStatusInvalid
	}
	if task.AssigneeAgentID == nil || *task.AssigneeAgentID != actor.Agent.AgentID {
		return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
	}
	if task.ActiveRunID == nil || *task.ActiveRunID != runID {
		return TaskRecord{}, ErrTaskActiveRunMismatch
	}
	if err := s.enforceRunContextGate(ctx, runID, "task.submit"); err != nil {
		return TaskRecord{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("begin submit tx failed: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'submitted',
			result = $3,
			updated_at = NOW()
		WHERE project_id = $1 AND id = $2
	`, projectID, taskID, nullableString(strings.TrimSpace(input.ResultMarkdown))); err != nil {
		return TaskRecord{}, fmt.Errorf("submit task update failed: %w", err)
	}

	for _, artifact := range input.Artifacts {
		if strings.TrimSpace(artifact.ArtifactType) == "" {
			continue
		}
		artifactID := ids.New(ids.PrefixArtifact)
		metadata := artifact.Metadata
		if metadata == nil {
			metadata = map[string]any{}
		}
		rawMeta, _ := json.Marshal(metadata)
		name := strings.TrimSpace(artifact.Name)
		if name == "" {
			name = artifactID
		}
		uri := strings.TrimSpace(artifact.URI)
		path := uri
		content := strings.TrimSpace(artifact.Content)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO artifacts (
				id, project_id, session_id, task_id, artifact_type, name, path, content, metadata, created_by_agent_id, created_by_run_id
			)
			SELECT $1, t.project_id, t.session_id, t.id, $2, $3, $4, $5, $6, $7, $8
			FROM tasks t
			WHERE t.id = $9 AND t.project_id = $10
		`, artifactID, normalizeArtifactType(artifact.ArtifactType), name, nullableString(path), nullableString(content), rawMeta, actor.Agent.AgentID, runID, taskID, projectID); err != nil {
			return TaskRecord{}, fmt.Errorf("insert artifact failed: %w", err)
		}
	}

	if err := s.ensureReviewTaskTx(ctx, tx, projectID, taskID, actor); err != nil {
		return TaskRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return TaskRecord{}, fmt.Errorf("submit task commit failed: %w", err)
	}
	committed = true

	s.recordEventBestEffort(ctx, projectID, "", taskID, "task.submitted", string(StatusRunning), string(StatusSubmitted), actor, map[string]any{"runId": runID})

	if s.memoryWriter != nil {
		summary := strings.TrimSpace(input.ResultMarkdown)
		if summary == "" {
			summary = "Task submitted without result markdown."
		}
		if _, writeErr := s.memoryWriter.Write(ctx, projectID, openviking.WriteInput{
			Target:        "summary",
			Title:         fmt.Sprintf("task-%s-summary", taskID),
			Content:       summary,
			RelatedTaskID: taskID,
			AutoSync:      true,
		}); writeErr != nil {
			s.logger.Warn("task submit summary write failed", "projectId", projectID, "taskId", taskID, "error", writeErr)
		}
	}

	return s.Get(ctx, projectID, taskID)
}

func (s *Service) Review(ctx context.Context, actor identity.Identity, projectID string, reviewTaskID string, input ReviewTaskInput) (TaskRecord, error) {
	if !actor.IsAgent() && !actor.IsOperator() {
		return TaskRecord{}, ErrReviewScopeRequired
	}
	if actor.IsAgent() && !actor.Agent.HasScope("task:review") {
		return TaskRecord{}, ErrReviewScopeRequired
	}
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return TaskRecord{}, err
	}

	reviewTask, err := s.Get(ctx, projectID, reviewTaskID)
	if err != nil {
		return TaskRecord{}, err
	}
	if reviewTask.ParentTaskID == nil || *reviewTask.ParentTaskID == "" {
		return TaskRecord{}, ErrTaskStatusInvalid
	}
	parentTaskID := *reviewTask.ParentTaskID
	parentTask, err := s.Get(ctx, projectID, parentTaskID)
	if err != nil {
		return TaskRecord{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("begin review tx failed: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	nextParentStatus := "done"
	parentError := nullableString("")
	if !input.Approve {
		nextParentStatus = "delegated"
		parentError = nullableString(strings.TrimSpace(input.Reason))
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET status = 'done', result = $3, updated_at = NOW() WHERE project_id = $1 AND id = $2`, projectID, reviewTaskID, nullableString(strings.TrimSpace(input.Reason))); err != nil {
		return TaskRecord{}, fmt.Errorf("update review task failed: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = $3,
			error = $4,
			active_run_id = NULL,
			started_at = NULL,
			updated_at = NOW()
		WHERE project_id = $1 AND id = $2
	`, projectID, parentTaskID, nextParentStatus, parentError); err != nil {
		return TaskRecord{}, fmt.Errorf("update parent task review result failed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return TaskRecord{}, fmt.Errorf("commit review tx failed: %w", err)
	}
	committed = true

	toStatus := StatusDone
	if !input.Approve {
		toStatus = StatusDelegated
	}
	s.recordEventBestEffort(ctx, projectID, "", parentTaskID, "task.reviewed", string(parentTask.Status), string(toStatus), actor, map[string]any{"approve": input.Approve, "reason": strings.TrimSpace(input.Reason)})

	return s.Get(ctx, projectID, parentTaskID)
}

func (s *Service) Fail(ctx context.Context, actor identity.Identity, projectID string, taskID string, input FailTaskInput) (TaskRecord, error) {
	if !actor.IsAgent() {
		return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
	}
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return TaskRecord{}, err
	}
	task, err := s.Get(ctx, projectID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	if state, err := s.runContextState(ctx, strings.TrimSpace(input.RunID)); err == nil && state == "handoff_only" {
		return TaskRecord{}, ErrContextHandoffRequired
	}
	if task.AssigneeAgentID == nil || *task.AssigneeAgentID != actor.Agent.AgentID {
		return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
	}
	if task.ActiveRunID == nil || strings.TrimSpace(input.RunID) == "" || *task.ActiveRunID != strings.TrimSpace(input.RunID) {
		return TaskRecord{}, ErrTaskActiveRunMismatch
	}
	if task.Status != StatusRunning && task.Status != StatusDelegated {
		return TaskRecord{}, ErrTaskStatusInvalid
	}

	var attemptCount, maxAttempts int
	err = s.db.QueryRowContext(ctx, `
		SELECT attempt_count + 1, max_attempts
		FROM tasks
		WHERE project_id = $1 AND id = $2
	`, projectID, taskID).Scan(&attemptCount, &maxAttempts)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("load attempts failed: %w", err)
	}

	nextStatus := "delegated"
	if attemptCount >= maxAttempts {
		nextStatus = "failed"
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = $3,
			attempt_count = attempt_count + 1,
			error = $4,
			active_run_id = CASE WHEN $3 = 'failed' THEN active_run_id ELSE NULL END,
			updated_at = NOW()
		WHERE project_id = $1 AND id = $2
	`, projectID, taskID, nextStatus, nullableString(strings.TrimSpace(input.Reason))); err != nil {
		return TaskRecord{}, fmt.Errorf("fail task update failed: %w", err)
	}

	s.recordEventBestEffort(ctx, projectID, "", taskID, "task.failed", string(task.Status), nextStatus, actor, map[string]any{"reason": strings.TrimSpace(input.Reason), "attemptCount": attemptCount, "maxAttempts": maxAttempts})
	return s.Get(ctx, projectID, taskID)
}

func (s *Service) Resume(ctx context.Context, actor identity.Identity, projectID string, taskID string, input ResumeTaskInput) (TaskRecord, error) {
	if !actor.IsAgent() {
		return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
	}
	handoffID := strings.TrimSpace(input.HandoffID)
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		return TaskRecord{}, newInvalidInputError(map[string]string{"runId": "cannot be empty"})
	}
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return TaskRecord{}, err
	}
	if state, err := s.runContextState(ctx, runID); err == nil && state == "handoff_only" {
		return TaskRecord{}, ErrContextHandoffRequired
	}

	task, err := s.Get(ctx, projectID, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	if task.AssigneeAgentID == nil || *task.AssigneeAgentID != actor.Agent.AgentID {
		return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
	}

	if handoffID != "" {
		var handoff struct {
			ProjectID   string
			TaskID      sql.NullString
			ToAgentID   sql.NullString
			FromAgentID string
			ConsumedAt  sql.NullTime
		}
		err = s.db.QueryRowContext(ctx, `
			SELECT project_id, task_id, to_agent_id, from_agent_id, consumed_at
			FROM context_handoffs
			WHERE id = $1
		`, handoffID).Scan(&handoff.ProjectID, &handoff.TaskID, &handoff.ToAgentID, &handoff.FromAgentID, &handoff.ConsumedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return TaskRecord{}, ErrHandoffNotFound
		}
		if err != nil {
			return TaskRecord{}, fmt.Errorf("load handoff failed: %w", err)
		}
		if handoff.ProjectID != projectID {
			return TaskRecord{}, ErrProjectAccessDenied
		}
		if handoff.ConsumedAt.Valid {
			return TaskRecord{}, ErrHandoffAlreadyConsumed
		}
		if handoff.TaskID.Valid && handoff.TaskID.String != taskID {
			return TaskRecord{}, ErrTaskStatusInvalid
		}
		if handoff.FromAgentID != actor.Agent.AgentID {
			return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
		}
		if handoff.ToAgentID.Valid && handoff.ToAgentID.String != actor.Agent.AgentID {
			return TaskRecord{}, ErrTaskNotAssignedToCurrentAgent
		}
	} else if ok, err := s.canResumeHeartbeatBlockedTask(ctx, projectID, taskID, task); err != nil {
		return TaskRecord{}, err
	} else if !ok {
		return TaskRecord{}, ErrTaskStatusInvalid
	}

	if err := s.ensureRunExists(ctx, runID, actor.Agent.AgentID, projectID, task); err != nil {
		return TaskRecord{}, err
	}
	if err := s.enforceRunContextGate(ctx, runID, "task.resume"); err != nil {
		return TaskRecord{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("begin resume tx failed: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	previousRunID := ""
	if task.ActiveRunID != nil {
		previousRunID = strings.TrimSpace(*task.ActiveRunID)
	}

	if handoffID != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE context_handoffs
			SET consumed_by_run_id = $2,
				consumed_at = NOW()
			WHERE id = $1 AND consumed_at IS NULL
		`, handoffID, runID); err != nil {
			return TaskRecord{}, fmt.Errorf("consume handoff failed: %w", err)
		}
	}

	if previousRunID != "" && previousRunID != runID {
		if _, err := tx.ExecContext(ctx, `
			UPDATE agent_runs
			SET status = 'ended',
				ended_at = NOW(),
				end_reason = 'superseded_by_resume'
			WHERE id = $1 AND status <> 'ended'
		`, previousRunID); err != nil {
			return TaskRecord{}, fmt.Errorf("end previous run failed: %w", err)
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'running',
			active_run_id = $3,
			error = NULL,
			started_at = NOW(),
			last_heartbeat_at = NOW(),
			updated_at = NOW()
		WHERE project_id = $1 AND id = $2 AND assignee_agent_id = $4
	`, projectID, taskID, runID, actor.Agent.AgentID)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("resume task update failed: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return TaskRecord{}, ErrTaskStatusInvalid
	}

	if err := tx.Commit(); err != nil {
		return TaskRecord{}, fmt.Errorf("commit resume tx failed: %w", err)
	}
	committed = true

	payload := map[string]any{"runId": runID}
	if handoffID != "" {
		payload["handoffId"] = handoffID
	} else {
		payload["recoveryReason"] = heartbeatTimeoutBlockReason
	}
	s.recordEventBestEffort(ctx, projectID, "", taskID, "task.resumed", string(task.Status), string(StatusRunning), actor, payload)
	return s.Get(ctx, projectID, taskID)
}

func (s *Service) ListArtifacts(ctx context.Context, projectID string, taskIDFilter string, typeFilter string) ([]ArtifactSummary, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}

	query := `
		SELECT id, task_id, artifact_type, name, COALESCE(path, ''), COALESCE(content, ''), COALESCE(metadata, '{}'::jsonb), created_at
		FROM artifacts
		WHERE project_id = $1
	`
	args := []any{projectID}
	idx := 2
	if taskID := strings.TrimSpace(taskIDFilter); taskID != "" {
		query += fmt.Sprintf(" AND task_id = $%d", idx)
		args = append(args, taskID)
		idx++
	}
	if artifactType := strings.TrimSpace(typeFilter); artifactType != "" {
		query += fmt.Sprintf(" AND artifact_type = $%d", idx)
		args = append(args, artifactType)
		idx++
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list artifacts query failed: %w", err)
	}
	defer rows.Close()

	items := make([]ArtifactSummary, 0)
	for rows.Next() {
		var item ArtifactSummary
		var taskID sql.NullString
		var rawMeta []byte
		if err := rows.Scan(&item.ArtifactID, &taskID, &item.ArtifactType, &item.Name, &item.Path, &item.Content, &rawMeta, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan artifact row failed: %w", err)
		}
		if taskID.Valid {
			value := taskID.String
			item.TaskID = &value
		}
		if len(rawMeta) > 0 {
			_ = json.Unmarshal(rawMeta, &item.Metadata)
		}
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifacts rows failed: %w", err)
	}
	return items, nil
}

func (s *Service) GetArtifact(ctx context.Context, projectID string, artifactID string) (ArtifactSummary, error) {
	projectID = strings.TrimSpace(projectID)
	artifactID = strings.TrimSpace(artifactID)
	if projectID == "" || artifactID == "" {
		return ArtifactSummary{}, newInvalidInputError(map[string]string{"projectId": "cannot be empty", "artifactId": "cannot be empty"})
	}

	var item ArtifactSummary
	var taskID sql.NullString
	var rawMeta []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, artifact_type, name, COALESCE(path, ''), COALESCE(content, ''), COALESCE(metadata, '{}'::jsonb), created_at
		FROM artifacts
		WHERE project_id = $1 AND id = $2
	`, projectID, artifactID).Scan(&item.ArtifactID, &taskID, &item.ArtifactType, &item.Name, &item.Path, &item.Content, &rawMeta, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ArtifactSummary{}, ErrTaskNotFound
	}
	if err != nil {
		return ArtifactSummary{}, fmt.Errorf("get artifact query failed: %w", err)
	}
	if taskID.Valid {
		value := taskID.String
		item.TaskID = &value
	}
	if len(rawMeta) > 0 {
		_ = json.Unmarshal(rawMeta, &item.Metadata)
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return item, nil
}

func (s *Service) loadDependencies(ctx context.Context, taskID string) ([]string, error) {
	return s.loadStrings(ctx, `SELECT depends_on_task_id FROM task_dependencies WHERE task_id = $1 ORDER BY depends_on_task_id ASC`, taskID)
}

func (s *Service) loadSkills(ctx context.Context, taskID string) ([]string, error) {
	return s.loadStrings(ctx, `SELECT skill_name FROM task_required_skills WHERE task_id = $1 ORDER BY skill_name ASC`, taskID)
}

func (s *Service) replaceTaskSkills(ctx context.Context, taskID string, skills []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace skills tx failed: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := s.replaceTaskSkillsTx(ctx, tx, taskID, skills); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace skills tx failed: %w", err)
	}
	committed = true
	return nil
}

func (s *Service) replaceTaskSkillsTx(ctx context.Context, tx *sql.Tx, taskID string, skills []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_required_skills WHERE task_id = $1`, taskID); err != nil {
		return fmt.Errorf("clear task skills failed: %w", err)
	}
	for _, skill := range uniqueStrings(skills) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_required_skills (task_id, skill_name) VALUES ($1, $2)`, taskID, skill); err != nil {
			return fmt.Errorf("insert task skill failed: %w", err)
		}
	}
	return nil
}

func (s *Service) replaceTaskDependenciesTx(ctx context.Context, tx *sql.Tx, projectID string, taskID string, deps []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_dependencies WHERE task_id = $1`, taskID); err != nil {
		return fmt.Errorf("clear task dependencies failed: %w", err)
	}
	for _, dep := range uniqueStrings(deps) {
		if dep == taskID {
			continue
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tasks WHERE id = $1 AND project_id = $2)`, dep, projectID).Scan(&exists); err != nil {
			return fmt.Errorf("check dependency exists failed: %w", err)
		}
		if !exists {
			return newInvalidInputError(map[string]string{"dependencies": "contains task outside current project or missing"})
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_dependencies (task_id, depends_on_task_id) VALUES ($1, $2)`, taskID, dep); err != nil {
			return fmt.Errorf("insert task dependency failed: %w", err)
		}
	}
	return nil
}

func (s *Service) insertDelegation(ctx context.Context, taskID string, assigneeAgentID string, actor identity.Identity, reason string) error {
	delegationID := ids.New(ids.PrefixDelegation)
	delegatedByType, delegatedByAgentID, delegatedByOperator := delegatedByFromActor(actor, s.consoleOperator)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO task_delegations (
			id, task_id, assignee_agent_id, delegated_by_type, delegated_by_agent_id, delegated_by_operator_label, reason
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, delegationID, taskID, assigneeAgentID, delegatedByType, delegatedByAgentID, delegatedByOperator, nullableString(reason))
	if err != nil {
		return fmt.Errorf("insert task delegation failed: %w", err)
	}
	return nil
}

func (s *Service) insertDelegationTx(ctx context.Context, tx *sql.Tx, taskID string, assigneeAgentID string, actor identity.Identity, reason string) error {
	delegationID := ids.New(ids.PrefixDelegation)
	delegatedByType, delegatedByAgentID, delegatedByOperator := delegatedByFromActor(actor, s.consoleOperator)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO task_delegations (
			id, task_id, assignee_agent_id, delegated_by_type, delegated_by_agent_id, delegated_by_operator_label, reason
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, delegationID, taskID, assigneeAgentID, delegatedByType, delegatedByAgentID, delegatedByOperator, nullableString(reason))
	if err != nil {
		return fmt.Errorf("insert task delegation failed: %w", err)
	}
	return nil
}

func (s *Service) loadStrings(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load string rows failed: %w", err)
	}
	defer rows.Close()
	items := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("scan string row failed: %w", err)
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate string rows failed: %w", err)
	}
	return items, nil
}

func (s *Service) activeSessionID(ctx context.Context, projectID string) (string, error) {
	var sessionID sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT active_session_id FROM projects WHERE id = $1`, projectID).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrProjectAccessDenied
	}
	if err != nil {
		return "", fmt.Errorf("query active session failed: %w", err)
	}
	if !sessionID.Valid || strings.TrimSpace(sessionID.String) == "" {
		return "", ErrTaskStatusInvalid
	}
	return sessionID.String, nil
}

func (s *Service) assertProjectWritable(ctx context.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM projects WHERE id = $1`, projectID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProjectAccessDenied
	}
	if err != nil {
		return fmt.Errorf("load project writable state failed: %w", err)
	}
	if strings.TrimSpace(status) == "archived" {
		return ErrProjectArchived
	}
	return nil
}

func (s *Service) dependenciesDone(ctx context.Context, taskID string) (bool, error) {
	var pendingCount int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM task_dependencies td
		JOIN tasks t ON t.id = td.depends_on_task_id
		WHERE td.task_id = $1 AND t.status <> 'done'
	`, taskID).Scan(&pendingCount)
	if err != nil {
		return false, fmt.Errorf("check dependencies done failed: %w", err)
	}
	return pendingCount == 0, nil
}

func (s *Service) ensureRunExists(ctx context.Context, runID string, agentID string, projectID string, task TaskRecord) error {
	var existing struct {
		AgentID   string
		ProjectID string
	}
	err := s.db.QueryRowContext(ctx, `SELECT agent_id, project_id FROM agent_runs WHERE id = $1`, runID).Scan(&existing.AgentID, &existing.ProjectID)
	if err == nil {
		if existing.AgentID != agentID || existing.ProjectID != projectID {
			return ErrTaskActiveRunMismatch
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check run exists failed: %w", err)
	}
	maxContextTokens := defaultMaxContextTokensForModel(valueOrEmpty(task.RequiredModel))
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_runs (
			id, agent_id, project_id, session_id, status,
			model_name, max_context_tokens, estimated_used_tokens, context_state,
			started_at
		)
		VALUES (
			$1, $2, $3, (SELECT session_id FROM tasks WHERE id = $4 AND project_id = $3), 'active',
			$5, $6, 0, 'normal',
			NOW()
		)
	`, runID, agentID, projectID, task.TaskID, nullableString(valueOrEmpty(task.RequiredModel)), maxContextTokens); err != nil {
		return fmt.Errorf("insert agent run failed: %w", err)
	}
	return nil
}

func (s *Service) enforceRunContextGate(ctx context.Context, runID string, operation string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	state, err := s.runContextState(ctx, runID)
	if err != nil {
		return err
	}
	switch state {
	case "handoff_required", "handoff_only":
		if operation == "task.heartbeat" {
			return nil
		}
		return ErrContextHandoffRequired
	default:
		return nil
	}
}

func (s *Service) runContextState(ctx context.Context, runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", nil
	}
	var state sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT context_state FROM agent_runs WHERE id = $1`, runID).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("load run context state failed: %w", err)
	}
	return strings.TrimSpace(state.String), nil
}

func (s *Service) canResumeHeartbeatBlockedTask(ctx context.Context, projectID string, taskID string, task TaskRecord) (bool, error) {
	if task.Status != StatusBlocked || task.ActiveRunID != nil {
		return false, nil
	}

	var reason string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(error, '') FROM tasks WHERE project_id = $1 AND id = $2`, projectID, taskID).Scan(&reason)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrTaskNotFound
	}
	if err != nil {
		return false, fmt.Errorf("load task blocked reason failed: %w", err)
	}
	if strings.TrimSpace(reason) != heartbeatTimeoutBlockReason {
		return false, nil
	}

	depsDone, err := s.dependenciesDone(ctx, task.TaskID)
	if err != nil {
		return false, err
	}
	if !depsDone {
		return false, ErrTaskDependencyNotDone
	}
	return true, nil
}

func (s *Service) validateAssignee(ctx context.Context, projectID string, agentID string, agentType string, requiredSkills []string, requiredModel string) (string, error) {
	var actualType string
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT agent_type, status FROM agents WHERE id = $1`, agentID).Scan(&actualType, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTaskNotEligibleForAgent
	}
	if err != nil {
		return "", fmt.Errorf("load assignee failed: %w", err)
	}
	if strings.TrimSpace(status) != "active" {
		return "", ErrTaskNotEligibleForAgent
	}
	if agentType != "" && agentType != actualType {
		return "", ErrTaskNotEligibleForAgent
	}
	var bound bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_project_bindings WHERE agent_id = $1 AND project_id = $2 AND enabled = TRUE)`, agentID, projectID).Scan(&bound); err != nil {
		return "", fmt.Errorf("check agent project binding failed: %w", err)
	}
	if !bound {
		return "", ErrAgentNotBoundToProject
	}

	for _, skill := range uniqueStrings(requiredSkills) {
		var hasSkill bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_skills WHERE agent_id = $1 AND skill_name = $2)`, agentID, skill).Scan(&hasSkill); err != nil {
			return "", fmt.Errorf("check agent skill failed: %w", err)
		}
		if !hasSkill {
			return "", ErrTaskNotEligibleForAgent
		}
	}

	if model := strings.TrimSpace(requiredModel); model != "" {
		var hasModel bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_models WHERE agent_id = $1 AND model_name = $2)`, agentID, model).Scan(&hasModel); err != nil {
			return "", fmt.Errorf("check agent model failed: %w", err)
		}
		if !hasModel {
			return "", ErrTaskNotEligibleForAgent
		}
	}

	return actualType, nil
}

func (s *Service) ListTaskEvents(ctx context.Context, projectID string, taskID string, limit int, beforeEventID string) ([]TaskEvent, *string, error) {
	projectID = strings.TrimSpace(projectID)
	taskID = strings.TrimSpace(taskID)
	if projectID == "" || taskID == "" {
		return nil, nil, newInvalidInputError(map[string]string{"projectId": "cannot be empty", "taskId": "cannot be empty"})
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, project_id, session_id, task_id, event_type,
			from_status, to_status,
			actor_type, actor_operator_label, actor_agent_id, actor_run_id,
			COALESCE(payload, '{}'::jsonb), created_at
		FROM task_events
		WHERE project_id = $1 AND task_id = $2
	`
	args := []any{projectID, taskID}
	if before := strings.TrimSpace(beforeEventID); before != "" {
		var beforeAt time.Time
		err := s.db.QueryRowContext(ctx, `SELECT created_at FROM task_events WHERE project_id = $1 AND task_id = $2 AND id = $3`, projectID, taskID, before).Scan(&beforeAt)
		if err == nil {
			query += ` AND (created_at < $3 OR (created_at = $3 AND id < $4))`
			args = append(args, beforeAt, before)
		}
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("list task events failed: %w", err)
	}
	defer rows.Close()

	items := make([]TaskEvent, 0)
	for rows.Next() {
		var item TaskEvent
		var taskIDValue sql.NullString
		var fromStatus sql.NullString
		var toStatus sql.NullString
		var actorOperator sql.NullString
		var actorAgent sql.NullString
		var actorRun sql.NullString
		var rawPayload []byte
		if err := rows.Scan(
			&item.EventID,
			&item.ProjectID,
			&item.SessionID,
			&taskIDValue,
			&item.EventType,
			&fromStatus,
			&toStatus,
			&item.ActorType,
			&actorOperator,
			&actorAgent,
			&actorRun,
			&rawPayload,
			&item.CreatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan task event failed: %w", err)
		}
		if taskIDValue.Valid {
			value := strings.TrimSpace(taskIDValue.String)
			item.TaskID = &value
		}
		if fromStatus.Valid {
			value := strings.TrimSpace(fromStatus.String)
			item.FromStatus = &value
		}
		if toStatus.Valid {
			value := strings.TrimSpace(toStatus.String)
			item.ToStatus = &value
		}
		if actorOperator.Valid {
			value := strings.TrimSpace(actorOperator.String)
			item.ActorOperatorLabel = &value
		}
		if actorAgent.Valid {
			value := strings.TrimSpace(actorAgent.String)
			item.ActorAgentID = &value
		}
		if actorRun.Valid {
			value := strings.TrimSpace(actorRun.String)
			item.ActorRunID = &value
		}
		if len(rawPayload) > 0 {
			_ = json.Unmarshal(rawPayload, &item.Payload)
		}
		if item.Payload == nil {
			item.Payload = map[string]any{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate task events failed: %w", err)
	}

	nextCursor := (*string)(nil)
	if len(items) == limit {
		value := items[len(items)-1].EventID
		nextCursor = &value
	}
	return items, nextCursor, nil
}

func (s *Service) ensureReviewTaskTx(ctx context.Context, tx *sql.Tx, projectID string, parentTaskID string, actor identity.Identity) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM tasks
			WHERE project_id = $1 AND parent_task_id = $2 AND created_by_type = 'system' AND title = 'Review submitted task'
		)
	`, projectID, parentTaskID).Scan(&exists); err != nil {
		return fmt.Errorf("check existing review task failed: %w", err)
	}
	if exists {
		return nil
	}

	var reviewerAgentID sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT a.id
		FROM agents a
		JOIN agent_skills s ON s.agent_id = a.id
		JOIN agent_project_bindings b ON b.agent_id = a.id AND b.project_id = $1 AND b.enabled = TRUE
		WHERE a.agent_type = $2 AND a.status = 'active' AND s.skill_name = $3
		ORDER BY a.created_at ASC
		LIMIT 1
	`, projectID, s.reviewerAgentType, s.reviewerSkill).Scan(&reviewerAgentID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("find reviewer agent failed: %w", err)
	}

	var parent struct {
		SessionID string
		Title     string
	}
	if err := tx.QueryRowContext(ctx, `SELECT session_id, title FROM tasks WHERE id = $1 AND project_id = $2`, parentTaskID, projectID).Scan(&parent.SessionID, &parent.Title); err != nil {
		return fmt.Errorf("load parent task for review failed: %w", err)
	}

	reviewTaskID := ids.New(ids.PrefixTask)
	status := "planned"
	var assignee any
	var assigneeType any
	var delegatedByType any
	var delegatedByAgent any
	var delegatedByOperator any
	var delegatedAt any
	if reviewerAgentID.Valid {
		status = "delegated"
		assignee = reviewerAgentID.String
		assigneeType = s.reviewerAgentType
		delegatedByType, delegatedByAgent, delegatedByOperator = delegatedByFromActor(identity.System(), s.consoleOperator)
		delegatedAt = time.Now().UTC()
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tasks (
			id, project_id, session_id, parent_task_id, title, description,
			status, assignee_agent_id, assignee_agent_type,
			delegated_by_type, delegated_by_agent_id, delegated_by_operator_label, delegated_at,
			created_by_type, created_by_operator_label, created_by_agent_id,
			is_required
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'system',NULL,NULL,FALSE)
	`, reviewTaskID, projectID, parent.SessionID, parentTaskID, "Review submitted task", "Review and approve/reject submitted task", status, assignee, assigneeType, delegatedByType, delegatedByAgent, delegatedByOperator, delegatedAt); err != nil {
		return fmt.Errorf("create review task failed: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO task_required_skills (task_id, skill_name) VALUES ($1, $2)`, reviewTaskID, s.reviewerSkill); err != nil {
		return fmt.Errorf("create review task skills failed: %w", err)
	}

	s.recordEventBestEffort(ctx, projectID, parent.SessionID, reviewTaskID, "task.review_task_created", "", status, actor, map[string]any{"parentTaskId": parentTaskID})
	return nil
}

func (s *Service) BlockTimedOutRunningTasks(ctx context.Context, heartbeatTimeout time.Duration, limit int) (int, error) {
	if heartbeatTimeout <= 0 {
		heartbeatTimeout = 90 * time.Second
	}
	if limit <= 0 {
		limit = 200
	}
	staleBefore := time.Now().UTC().Add(-heartbeatTimeout)

	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.project_id, t.session_id, COALESCE(t.active_run_id, '')
		FROM tasks t
		LEFT JOIN agent_runs ar ON ar.id = t.active_run_id
		WHERE t.status = 'running'
		  AND GREATEST(
				COALESCE(t.last_heartbeat_at, t.updated_at),
				COALESCE(ar.last_heartbeat_at, ar.started_at, t.updated_at)
			  ) < $1
		ORDER BY GREATEST(
				COALESCE(t.last_heartbeat_at, t.updated_at),
				COALESCE(ar.last_heartbeat_at, ar.started_at, t.updated_at)
			  ) ASC
		LIMIT $2
	`, staleBefore, limit)
	if err != nil {
		return 0, fmt.Errorf("list timed out running tasks failed: %w", err)
	}
	defer rows.Close()

	type staleTask struct {
		TaskID    string
		ProjectID string
		SessionID string
		RunID     string
	}
	stale := make([]staleTask, 0)
	for rows.Next() {
		var item staleTask
		if err := rows.Scan(&item.TaskID, &item.ProjectID, &item.SessionID, &item.RunID); err != nil {
			return 0, fmt.Errorf("scan timed out task failed: %w", err)
		}
		stale = append(stale, item)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate timed out tasks failed: %w", err)
	}

	blocked := 0
	for _, item := range stale {
		result, err := s.db.ExecContext(ctx, `
			UPDATE tasks
			SET status = 'blocked',
				error = $3,
				active_run_id = NULL,
				updated_at = NOW()
			WHERE project_id = $1
			  AND id = $2
			  AND status = 'running'
		`, item.ProjectID, item.TaskID, heartbeatTimeoutBlockReason)
		if err != nil {
			return blocked, fmt.Errorf("block timed out task failed: %w", err)
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			continue
		}

		if strings.TrimSpace(item.RunID) != "" {
			_, _ = s.db.ExecContext(ctx, `
				UPDATE agent_runs
				SET status = 'ended',
					ended_at = NOW(),
					end_reason = 'heartbeat_timeout'
				WHERE id = $1
				  AND status <> 'ended'
			`, item.RunID)
		}

		s.recordEventBestEffort(ctx, item.ProjectID, item.SessionID, item.TaskID, "task.blocked", string(StatusRunning), string(StatusBlocked), identity.System(), map[string]any{
			"reason":                 "active_run_heartbeat_timeout",
			"timeoutSeconds":         int(heartbeatTimeout.Seconds()),
			"timedOutActiveRunId":    strings.TrimSpace(item.RunID),
			"blockedByMaintenance":   true,
			"maintenanceOperationId": "worker.active_run_timeout",
		})
		blocked++
	}

	return blocked, nil
}

func (s *Service) EnsureSubmittedReviewTasks(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.project_id, t.id
		FROM tasks t
		WHERE t.status = 'submitted'
		  AND NOT EXISTS (
		    SELECT 1
		    FROM tasks r
		    WHERE r.project_id = t.project_id
		      AND r.parent_task_id = t.id
		      AND r.created_by_type = 'system'
		      AND r.title = 'Review submitted task'
		  )
		ORDER BY t.updated_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("list submitted tasks without review failed: %w", err)
	}
	defer rows.Close()

	type submittedTask struct {
		ProjectID string
		TaskID    string
	}
	pending := make([]submittedTask, 0)
	for rows.Next() {
		var item submittedTask
		if err := rows.Scan(&item.ProjectID, &item.TaskID); err != nil {
			return 0, fmt.Errorf("scan submitted task failed: %w", err)
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate submitted task rows failed: %w", err)
	}

	created := 0
	for _, item := range pending {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return created, fmt.Errorf("begin submitted review tx failed: %w", err)
		}
		committed := false
		var opErr error
		func() {
			defer func() {
				if !committed {
					_ = tx.Rollback()
				}
			}()
			if ensureErr := s.ensureReviewTaskTx(ctx, tx, item.ProjectID, item.TaskID, identity.System()); ensureErr != nil {
				opErr = ensureErr
				return
			}
			if commitErr := tx.Commit(); commitErr != nil {
				opErr = fmt.Errorf("commit submitted review tx failed: %w", commitErr)
				return
			}
			committed = true
		}()
		if opErr != nil {
			return created, opErr
		}
		created++
	}

	return created, nil
}

func (s *Service) SyncTaskSummaries(ctx context.Context, limit int) (int, error) {
	if s.memoryWriter == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 200
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT t.project_id, t.id, t.status, COALESCE(t.result, ''), COALESCE(t.session_id, '')
		FROM tasks t
		WHERE t.status IN ('submitted', 'done')
		  AND COALESCE(TRIM(t.result), '') <> ''
		  AND NOT EXISTS (
		    SELECT 1
		    FROM task_events te
		    WHERE te.task_id = t.id
		      AND te.event_type = 'task.summary_synced'
		  )
		ORDER BY t.updated_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("list unsynced task summaries failed: %w", err)
	}
	defer rows.Close()

	type summaryTask struct {
		ProjectID string
		TaskID    string
		Status    string
		Result    string
		SessionID string
	}
	items := make([]summaryTask, 0)
	for rows.Next() {
		var item summaryTask
		if err := rows.Scan(&item.ProjectID, &item.TaskID, &item.Status, &item.Result, &item.SessionID); err != nil {
			return 0, fmt.Errorf("scan unsynced task summary failed: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate unsynced task summaries failed: %w", err)
	}

	synced := 0
	for _, item := range items {
		if _, err := s.memoryWriter.Write(ctx, item.ProjectID, openviking.WriteInput{
			Target:        "summary",
			Title:         fmt.Sprintf("task-%s-summary", item.TaskID),
			Content:       strings.TrimSpace(item.Result),
			RelatedTaskID: item.TaskID,
			AutoSync:      true,
		}); err != nil {
			s.logger.Warn("worker task summary sync failed", "projectId", item.ProjectID, "taskId", item.TaskID, "error", err)
			continue
		}
		s.recordEventBestEffort(ctx, item.ProjectID, item.SessionID, item.TaskID, "task.summary_synced", item.Status, item.Status, identity.System(), map[string]any{
			"syncedBy": "worker",
		})
		synced++
	}
	return synced, nil
}

func (s *Service) recordEventBestEffort(
	ctx context.Context,
	projectID string,
	sessionID string,
	taskID string,
	eventType string,
	fromStatus string,
	toStatus string,
	actor identity.Identity,
	payload map[string]any,
) {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(taskID) == "" {
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		_ = s.db.QueryRowContext(ctx, `SELECT session_id FROM tasks WHERE id = $1 AND project_id = $2`, taskID, projectID).Scan(&sessionID)
	}

	rawPayload, _ := json.Marshal(payload)
	eventID := ids.New("evt")
	createdAt := time.Now().UTC()
	actorType := string(actor.SenderType)
	actorOperator := any(nil)
	actorAgent := any(nil)
	if actor.IsOperator() {
		actorOperator = nullableString(actor.OperatorLabel)
	} else if actor.IsAgent() {
		actorAgent = nullableString(actor.Agent.AgentID)
	}
	if actorType == "" {
		actorType = string(identity.SenderTypeSystem)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO task_events (
			id, project_id, session_id, task_id, event_type,
			from_status, to_status,
			actor_type, actor_operator_label, actor_agent_id,
			payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, eventID, projectID, nullableString(sessionID), taskID, eventType, nullableString(fromStatus), nullableString(toStatus), actorType, actorOperator, actorAgent, rawPayload); err != nil {
		s.logger.Warn("task event write failed", "projectId", projectID, "taskId", taskID, "eventType", eventType, "error", err)
	}
	if s.eventSink != nil {
		s.eventSink.PublishTaskEvent(ctx, eventID, projectID, taskID, eventType, fromStatus, toStatus, actor, payload, createdAt)
	}
}

func scanTaskRow(scanner interface {
	Scan(dest ...any) error
}) (TaskRecord, error) {
	var task TaskRecord
	var description sql.NullString
	var goal sql.NullString
	var inputs sql.NullString
	var constraints sql.NullString
	var parentTaskID sql.NullString
	var assigneeAgentID sql.NullString
	var assigneeAgentType sql.NullString
	var delegatedByType sql.NullString
	var delegatedByOperator sql.NullString
	var delegatedByAgent sql.NullString
	var delegatedAt sql.NullTime
	var activeRunID sql.NullString
	var lastHeartbeatAt sql.NullTime
	var requiredModel sql.NullString
	var outputContract sql.NullString

	err := scanner.Scan(
		&task.TaskID,
		&task.ProjectID,
		&task.Title,
		&description,
		&goal,
		&inputs,
		&constraints,
		&task.Status,
		&parentTaskID,
		&assigneeAgentID,
		&assigneeAgentType,
		&delegatedByType,
		&delegatedByOperator,
		&delegatedByAgent,
		&delegatedAt,
		&activeRunID,
		&lastHeartbeatAt,
		&requiredModel,
		&outputContract,
		&task.Priority,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		return TaskRecord{}, err
	}

	if description.Valid {
		task.Description = description.String
	}
	if goal.Valid {
		value := goal.String
		task.Goal = &value
	}
	if inputs.Valid {
		value := inputs.String
		task.Inputs = &value
	}
	if constraints.Valid {
		value := constraints.String
		task.Constraints = &value
	}
	if parentTaskID.Valid {
		value := parentTaskID.String
		task.ParentTaskID = &value
	}
	if assigneeAgentID.Valid {
		value := assigneeAgentID.String
		task.AssigneeAgentID = &value
	}
	if assigneeAgentType.Valid {
		value := assigneeAgentType.String
		task.AssigneeAgentType = &value
	}
	if delegatedByType.Valid {
		task.DelegatedByType = delegatedByType.String
	} else {
		task.DelegatedByType = "system"
	}
	if delegatedByOperator.Valid {
		value := delegatedByOperator.String
		task.DelegatedByOperatorLabel = &value
	}
	if delegatedByAgent.Valid {
		value := delegatedByAgent.String
		task.DelegatedByAgentID = &value
	}
	if delegatedAt.Valid {
		value := delegatedAt.Time
		task.DelegatedAt = &value
	}
	if activeRunID.Valid {
		value := activeRunID.String
		task.ActiveRunID = &value
	}
	if lastHeartbeatAt.Valid {
		value := lastHeartbeatAt.Time
		task.LastHeartbeatAt = &value
	}
	if requiredModel.Valid {
		value := requiredModel.String
		task.RequiredModel = &value
	}
	if outputContract.Valid {
		value := outputContract.String
		task.OutputContract = &value
	}
	if task.Dependencies == nil {
		task.Dependencies = []string{}
	}
	if task.RequiredSkills == nil {
		task.RequiredSkills = []string{}
	}
	return task, nil
}

func normalizeCreateInput(input CreateTaskInput) CreateTaskInput {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Goal = strings.TrimSpace(input.Goal)
	input.Inputs = strings.TrimSpace(input.Inputs)
	input.Constraints = strings.TrimSpace(input.Constraints)
	input.ParentTaskID = strings.TrimSpace(input.ParentTaskID)
	input.DelegateToAgentID = strings.TrimSpace(input.DelegateToAgentID)
	input.DelegateToAgentType = ""
	input.RequiredModel = strings.TrimSpace(input.RequiredModel)
	input.OutputContract = strings.TrimSpace(input.OutputContract)
	input.RequiredSkills = uniqueStrings(input.RequiredSkills)
	input.Dependencies = uniqueStrings(input.Dependencies)
	return input
}

func validateCreateInput(projectID string, input CreateTaskInput) map[string]string {
	details := map[string]string{}
	input.DelegateToAgentType = ""
	if strings.TrimSpace(projectID) == "" {
		details["projectId"] = "cannot be empty"
	}
	if n := utf8.RuneCountInString(input.Title); n < 2 || n > 180 {
		details["title"] = "must be between 2 and 180 characters"
	}
	if n := utf8.RuneCountInString(input.Description); n > 4000 {
		details["description"] = "must be at most 4000 characters"
	}
	if n := utf8.RuneCountInString(input.Goal); n > 400 {
		details["goal"] = "must be at most 400 characters"
	}
	if n := utf8.RuneCountInString(input.Inputs); n > 4000 {
		details["inputs"] = "must be at most 4000 characters"
	}
	if n := utf8.RuneCountInString(input.Constraints); n > 2000 {
		details["constraints"] = "must be at most 2000 characters"
	}
	if n := utf8.RuneCountInString(input.OutputContract); n > 4000 {
		details["outputContract"] = "must be at most 4000 characters"
	}
	if input.Priority < -10 || input.Priority > 10_000 {
		details["priority"] = "must be between -10 and 10000"
	}
	if input.DelegateToAgentType != "" && input.DelegateToAgentID == "" {
		details["delegateToAgentId"] = "cannot be empty when delegateToAgentType provided"
	}
	return details
}

func canEditTask(status Status) bool {
	switch status {
	case StatusPlanned, StatusDelegated, StatusBlocked:
		return true
	default:
		return false
	}
}

func ensureTransitionAllowed(from Status, to Status) error {
	allowed := map[Status]map[Status]struct{}{
		StatusPlanned: {
			StatusDelegated: {},
			StatusCancelled: {},
		},
		StatusDelegated: {
			StatusRunning:   {},
			StatusCancelled: {},
			StatusBlocked:   {},
			StatusFailed:    {},
		},
		StatusRunning: {
			StatusSubmitted: {},
			StatusCancelled: {},
			StatusFailed:    {},
			StatusBlocked:   {},
		},
		StatusSubmitted: {
			StatusReviewing: {},
			StatusDone:      {},
			StatusDelegated: {},
		},
		StatusReviewing: {
			StatusDone:      {},
			StatusDelegated: {},
		},
		StatusBlocked: {
			StatusDelegated: {},
			StatusCancelled: {},
		},
	}
	if transitions, ok := allowed[from]; ok {
		if _, ok := transitions[to]; ok {
			return nil
		}
	}
	return ErrTaskStatusInvalid
}

func delegatedByFromActor(actor identity.Identity, fallbackOperator string) (byType any, byAgent any, byOperator any) {
	switch actor.SenderType {
	case identity.SenderTypeAgent:
		return "agent", nullableString(actor.Agent.AgentID), nil
	case identity.SenderTypeSystem:
		return "system", nil, nil
	default:
		label := strings.TrimSpace(actor.OperatorLabel)
		if label == "" {
			label = fallbackOperator
		}
		return "operator", nil, nullableString(label)
	}
}

func createdByFromActor(actor identity.Identity, fallbackOperator string) (byType any, byAgent any, byOperator any) {
	switch actor.SenderType {
	case identity.SenderTypeAgent:
		return "agent", nullableString(actor.Agent.AgentID), nil
	case identity.SenderTypeSystem:
		return "system", nil, nil
	default:
		label := strings.TrimSpace(actor.OperatorLabel)
		if label == "" {
			label = fallbackOperator
		}
		return "operator", nil, nullableString(label)
	}
}

func uniqueStrings(items []string) []string {
	set := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		set[item] = struct{}{}
	}
	if len(set) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	return out
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeArtifactType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "report"
	}
	switch value {
	case "diff", "code_diff", "doc", "report", "image":
		if value == "diff" {
			return "code_diff"
		}
		return value
	default:
		return "report"
	}
}

func defaultMaxContextTokensForModel(model string) int {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(model, "claude"):
		return 200_000
	case strings.Contains(model, "gpt-4.1"), strings.Contains(model, "gpt-5"), strings.Contains(model, "o3"), strings.Contains(model, "o4"):
		return 128_000
	case strings.Contains(model, "gemini"):
		return 1_000_000
	default:
		return 200_000
	}
}

func (input CreateTaskInput) ReasonOrDefault() string {
	return "created with delegated assignee"
}

func (input DelegateTaskInput) reasonOrDefault() string {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return "delegated by operator"
	}
	return reason
}
