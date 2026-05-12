package projects

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/pkg/ids"
)

const (
	projectStatusActive    = "active"
	projectStatusCompleted = "completed"
	projectStatusArchived  = "archived"

	rootTaskStatusPlanned  = "planned"
	createdByTypeOperator  = "operator"
	defaultRootTaskTitle   = "Project bootstrap"
	rootTaskDescriptionFmt = "Coordinate project bootstrap and task delegation. Goal: %s"
)

var (
	defaultCompletionPolicyJSON = []byte(`{"requiredTasks":"all_required_done","blockedTasks":"none","failedTasks":"none","reviewPolicy":"all_submitted_reviewed"}`)
	ErrCreateProjectFailed      = errors.New("create project failed")
	ErrProjectNotFound          = errors.New("project not found")
	ErrProjectCompletionFailed  = errors.New("project completion policy check failed")
	ErrProjectArchived          = errors.New("project archived")
)

type InvalidInputError struct {
	details map[string]string
}

func NewInvalidInputError(details map[string]string) error {
	return &InvalidInputError{details: cloneStringMap(details)}
}

func NewCompletionPolicyFailedError(failedItems []CompletionPolicyResultItem) error {
	items := make([]CompletionPolicyResultItem, len(failedItems))
	copy(items, failedItems)
	return &CompletionPolicyFailedError{failedItems: items}
}

func (e *InvalidInputError) Error() string {
	return "invalid project input"
}

func (e *InvalidInputError) Details() map[string]string {
	return cloneStringMap(e.details)
}

type CompletionPolicyFailedError struct {
	failedItems []CompletionPolicyResultItem
}

func (e *CompletionPolicyFailedError) Error() string {
	return "project completion policy check failed"
}

func (e *CompletionPolicyFailedError) Unwrap() error {
	return ErrProjectCompletionFailed
}

func (e *CompletionPolicyFailedError) FailedItems() []CompletionPolicyResultItem {
	if len(e.failedItems) == 0 {
		return []CompletionPolicyResultItem{}
	}
	items := make([]CompletionPolicyResultItem, len(e.failedItems))
	copy(items, e.failedItems)
	return items
}

type CreateProjectHooks struct {
	InitializeOpenViking  func(ctx context.Context, tx *sql.Tx, payload HookPayload) error
	InitializeProjectRoom func(ctx context.Context, tx *sql.Tx, payload HookPayload) (string, error)
	BindDefaultAgents     func(ctx context.Context, tx *sql.Tx, payload HookPayload) error
}

type HookPayload struct {
	ProjectID      string
	SessionID      string
	RootTaskID     string
	RoomID         string
	OpenVikingRoot string
	Name           string
	Goal           string
	Description    string
}

type Options struct {
	DB                   *sql.DB
	ConsoleOperatorLabel string
	OpenVikingNamespace  string
	Hooks                CreateProjectHooks
	OnProjectCompleted   func(ctx context.Context, projectID string)
}

type Service struct {
	db                  *sql.DB
	operatorLabel       string
	openVikingNamespace string
	hooks               CreateProjectHooks
	onProjectCompleted  func(ctx context.Context, projectID string)
}

type CompletionPolicy struct {
	RequiredTasks string `json:"requiredTasks"`
	BlockedTasks  string `json:"blockedTasks"`
	FailedTasks   string `json:"failedTasks"`
	ReviewPolicy  string `json:"reviewPolicy,omitempty"`
}

type CompletionPolicyPatch struct {
	RequiredTasks *string `json:"requiredTasks"`
	BlockedTasks  *string `json:"blockedTasks"`
	FailedTasks   *string `json:"failedTasks"`
	ReviewPolicy  *string `json:"reviewPolicy"`
}

type ProjectProgress struct {
	Done    int
	Total   int
	Blocked int
}

type ProjectListItem struct {
	ProjectID string
	Name      string
	Status    string
	Progress  ProjectProgress
	UpdatedAt time.Time
}

type ListProjectsOutput struct {
	Items      []ProjectListItem
	NextCursor *string
}

type ProjectDetail struct {
	ProjectID             string
	Name                  string
	Goal                  string
	Description           string
	Status                string
	ActiveSessionID       string
	OpenVikingRoot        string
	OpenVikingNamespace   string
	OpenVikingWorkspaceID string
	RoomID                string
	OperatorLabel         string
	CompletionPolicy      CompletionPolicy
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type CreateProjectInput struct {
	Name        string
	Goal        string
	Description string
}

type CreateProjectOutput struct {
	ProjectID       string
	Name            string
	Status          string
	ActiveSessionID string
	OpenVikingRoot  string
	RoomID          string
	InitCommand     string
}

type UpdateProjectInput struct {
	Name                  *string
	Goal                  *string
	Description           *string
	CompletionPolicy      *CompletionPolicyPatch
	OpenVikingNamespace   *string
	OpenVikingWorkspaceID *string
}

type UpdateProjectOutput struct {
	ProjectID string
	Name      string
	Status    string
	UpdatedAt time.Time
}

type CompleteProjectInput struct {
	Confirm bool
}

type ArchiveProjectInput struct {
	Confirm bool
	Reason  string
}

type CompletionPolicyResultItem struct {
	Code    string
	Message string
	TaskID  string
}

type CompletionPolicyResult struct {
	Passed      bool
	FailedItems []CompletionPolicyResultItem
}

type CompletionCandidate struct {
	ProjectID string
	Name      string
}

type CompleteProjectOutput struct {
	ProjectID    string
	Status       string
	PolicyResult CompletionPolicyResult
}

type ArchiveProjectOutput struct {
	ProjectID string
	Status    string
	UpdatedAt time.Time
}

func New(options Options) (*Service, error) {
	if options.DB == nil {
		return nil, errors.New("projects service requires a database handle")
	}

	operatorLabel := strings.TrimSpace(options.ConsoleOperatorLabel)
	if operatorLabel == "" {
		return nil, errors.New("projects service requires a non-empty operator label")
	}

	namespace := strings.TrimSpace(options.OpenVikingNamespace)
	if namespace == "" {
		return nil, errors.New("projects service requires a non-empty openviking namespace")
	}

	return &Service{
		db:                  options.DB,
		operatorLabel:       operatorLabel,
		openVikingNamespace: namespace,
		hooks:               options.Hooks,
		onProjectCompleted:  options.OnProjectCompleted,
	}, nil
}

func (s *Service) Create(ctx context.Context, input CreateProjectInput) (CreateProjectOutput, error) {
	normalized := CreateProjectInput{
		Name:        strings.TrimSpace(input.Name),
		Goal:        strings.TrimSpace(input.Goal),
		Description: strings.TrimSpace(input.Description),
	}

	if details := validateCreateInput(normalized); len(details) > 0 {
		return CreateProjectOutput{}, NewInvalidInputError(details)
	}

	projectID := ids.New(ids.PrefixProject)
	sessionID := ids.New(ids.PrefixSession)
	rootTaskID := ids.New(ids.PrefixTask)
	roomID := ids.New(ids.PrefixRoom)
	openVikingRoot := fmt.Sprintf("viking://%s/projects/%s", s.openVikingNamespace, projectID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CreateProjectOutput{}, fmt.Errorf("%w: begin tx: %v", ErrCreateProjectFailed, err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO projects (
			id,
			name,
			description,
			goal,
			status,
			created_by_label,
			active_session_id,
			openviking_namespace,
			openviking_root_uri,
			completion_policy
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`,
		projectID,
		normalized.Name,
		nullableString(normalized.Description),
		normalized.Goal,
		projectStatusActive,
		s.operatorLabel,
		sessionID,
		s.openVikingNamespace,
		openVikingRoot,
		defaultCompletionPolicyJSON,
	); err != nil {
		return CreateProjectOutput{}, fmt.Errorf("%w: insert project: %v", ErrCreateProjectFailed, err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_sessions (
			id,
			project_id,
			status,
			current_phase
		) VALUES ($1,$2,$3,$4)
	`,
		sessionID,
		projectID,
		projectStatusActive,
		"project_bootstrap",
	); err != nil {
		return CreateProjectOutput{}, fmt.Errorf("%w: insert project session: %v", ErrCreateProjectFailed, err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tasks (
			id,
			project_id,
			session_id,
			title,
			description,
			status,
			created_by_type,
			created_by_operator_label
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`,
		rootTaskID,
		projectID,
		sessionID,
		defaultRootTaskTitle,
		fmt.Sprintf(rootTaskDescriptionFmt, normalized.Goal),
		rootTaskStatusPlanned,
		createdByTypeOperator,
		s.operatorLabel,
	); err != nil {
		return CreateProjectOutput{}, fmt.Errorf("%w: insert root task: %v", ErrCreateProjectFailed, err)
	}

	hookPayload := HookPayload{
		ProjectID:      projectID,
		SessionID:      sessionID,
		RootTaskID:     rootTaskID,
		RoomID:         roomID,
		OpenVikingRoot: openVikingRoot,
		Name:           normalized.Name,
		Goal:           normalized.Goal,
		Description:    normalized.Description,
	}

	if s.hooks.InitializeOpenViking != nil {
		if err := s.hooks.InitializeOpenViking(ctx, tx, hookPayload); err != nil {
			return CreateProjectOutput{}, fmt.Errorf("%w: initialize openviking: %w", ErrCreateProjectFailed, err)
		}
	}

	if s.hooks.InitializeProjectRoom != nil {
		hookRoomID, err := s.hooks.InitializeProjectRoom(ctx, tx, hookPayload)
		if err != nil {
			return CreateProjectOutput{}, fmt.Errorf("%w: initialize project room: %v", ErrCreateProjectFailed, err)
		}
		if strings.TrimSpace(hookRoomID) != "" {
			roomID = hookRoomID
			hookPayload.RoomID = roomID
		}
	}

	if s.hooks.BindDefaultAgents != nil {
		if err := s.hooks.BindDefaultAgents(ctx, tx, hookPayload); err != nil {
			return CreateProjectOutput{}, fmt.Errorf("%w: bind default agents: %v", ErrCreateProjectFailed, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return CreateProjectOutput{}, fmt.Errorf("%w: commit tx: %v", ErrCreateProjectFailed, err)
	}
	committed = true

	return CreateProjectOutput{
		ProjectID:       projectID,
		Name:            normalized.Name,
		Status:          projectStatusActive,
		ActiveSessionID: sessionID,
		OpenVikingRoot:  openVikingRoot,
		RoomID:          roomID,
		InitCommand:     fmt.Sprintf("aitask init --project %s", projectID),
	}, nil
}

func (s *Service) List(ctx context.Context) (ListProjectsOutput, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			p.id,
			p.name,
			p.status,
			p.updated_at,
			COALESCE(ps.done_count, COALESCE(COUNT(t.id) FILTER (WHERE t.status = 'done'), 0)) AS done_count,
			COALESCE(ps.total_count, COALESCE(COUNT(t.id), 0)) AS total_count,
			COALESCE(ps.blocked_count, COALESCE(COUNT(t.id) FILTER (WHERE t.status = 'blocked'), 0)) AS blocked_count
		FROM projects p
		LEFT JOIN project_progress_snapshots ps ON ps.project_id = p.id
		LEFT JOIN tasks t ON t.project_id = p.id AND ps.project_id IS NULL
		GROUP BY p.id, p.name, p.status, p.updated_at, ps.done_count, ps.total_count, ps.blocked_count
		ORDER BY p.updated_at DESC, p.id DESC
	`)
	if err != nil {
		return ListProjectsOutput{}, fmt.Errorf("list projects query failed: %w", err)
	}
	defer rows.Close()

	items := make([]ProjectListItem, 0)
	for rows.Next() {
		var item ProjectListItem
		if err := rows.Scan(
			&item.ProjectID,
			&item.Name,
			&item.Status,
			&item.UpdatedAt,
			&item.Progress.Done,
			&item.Progress.Total,
			&item.Progress.Blocked,
		); err != nil {
			return ListProjectsOutput{}, fmt.Errorf("scan project list row failed: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListProjectsOutput{}, fmt.Errorf("iterate project list rows failed: %w", err)
	}

	return ListProjectsOutput{
		Items:      items,
		NextCursor: nil,
	}, nil
}

func (s *Service) Get(ctx context.Context, projectID string) (ProjectDetail, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ProjectDetail{}, NewInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}

	var detail ProjectDetail
	var goal sql.NullString
	var description sql.NullString
	var activeSessionID sql.NullString
	var operatorLabel sql.NullString
	var roomID sql.NullString
	var completionPolicyRaw []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT
			p.id,
			p.name,
			p.goal,
			p.description,
			p.status,
			p.active_session_id,
			p.openviking_root_uri,
			p.openviking_namespace,
			p.openviking_workspace_id,
			COALESCE(r.id, '') AS room_id,
			p.created_by_label,
			p.completion_policy,
			p.created_at,
			p.updated_at
		FROM projects p
		LEFT JOIN project_rooms r ON r.project_id = p.id
		WHERE p.id = $1
	`, projectID).Scan(
		&detail.ProjectID,
		&detail.Name,
		&goal,
		&description,
		&detail.Status,
		&activeSessionID,
		&detail.OpenVikingRoot,
		&detail.OpenVikingNamespace,
		&detail.OpenVikingWorkspaceID,
		&roomID,
		&operatorLabel,
		&completionPolicyRaw,
		&detail.CreatedAt,
		&detail.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectDetail{}, ErrProjectNotFound
	}
	if err != nil {
		return ProjectDetail{}, fmt.Errorf("get project query failed: %w", err)
	}

	detail.Goal = nullableStringValue(goal)
	detail.Description = nullableStringValue(description)
	detail.ActiveSessionID = nullableStringValue(activeSessionID)
	detail.RoomID = nullableStringValue(roomID)
	detail.OperatorLabel = nullableStringValue(operatorLabel)

	policy, err := decodeCompletionPolicy(completionPolicyRaw)
	if err != nil {
		return ProjectDetail{}, fmt.Errorf("decode completion policy failed: %w", err)
	}
	detail.CompletionPolicy = policy

	return detail, nil
}

func (s *Service) Update(ctx context.Context, projectID string, input UpdateProjectInput) (UpdateProjectOutput, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return UpdateProjectOutput{}, NewInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}
	if input.Name == nil && input.Goal == nil && input.Description == nil && input.CompletionPolicy == nil && input.OpenVikingNamespace == nil && input.OpenVikingWorkspaceID == nil {
		return UpdateProjectOutput{}, NewInvalidInputError(map[string]string{"body": "at least one field must be provided"})
	}

	var existingName string
	var existingStatus string
	var existingCompletionPolicyRaw []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT name, status, completion_policy
		FROM projects
		WHERE id = $1
	`, projectID).Scan(&existingName, &existingStatus, &existingCompletionPolicyRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return UpdateProjectOutput{}, ErrProjectNotFound
	}
	if err != nil {
		return UpdateProjectOutput{}, fmt.Errorf("load existing project for update failed: %w", err)
	}
	if existingStatus == projectStatusArchived {
		return UpdateProjectOutput{}, ErrProjectArchived
	}

	fields := make([]string, 0, 4)
	args := make([]any, 0, 5)
	argPos := 1
	updatedName := existingName

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if n := utf8.RuneCountInString(name); n < 2 || n > 80 {
			return UpdateProjectOutput{}, NewInvalidInputError(map[string]string{"name": "must be between 2 and 80 characters"})
		}
		updatedName = name
		fields = append(fields, fmt.Sprintf("name = $%d", argPos))
		args = append(args, name)
		argPos++
	}

	if input.Goal != nil {
		goal := strings.TrimSpace(*input.Goal)
		if n := utf8.RuneCountInString(goal); n < 4 || n > 200 {
			return UpdateProjectOutput{}, NewInvalidInputError(map[string]string{"goal": "must be between 4 and 200 characters"})
		}
		fields = append(fields, fmt.Sprintf("goal = $%d", argPos))
		args = append(args, goal)
		argPos++
	}

	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		if n := utf8.RuneCountInString(description); n > 500 {
			return UpdateProjectOutput{}, NewInvalidInputError(map[string]string{"description": "must be at most 500 characters"})
		}
		fields = append(fields, fmt.Sprintf("description = $%d", argPos))
		args = append(args, nullableString(description))
		argPos++
	}

	if input.CompletionPolicy != nil {
		existingPolicy, err := decodeCompletionPolicy(existingCompletionPolicyRaw)
		if err != nil {
			return UpdateProjectOutput{}, fmt.Errorf("decode existing completion policy failed: %w", err)
		}
		mergedPolicy, details := applyCompletionPolicyPatch(existingPolicy, *input.CompletionPolicy)
		if len(details) > 0 {
			return UpdateProjectOutput{}, NewInvalidInputError(details)
		}
		policyJSON, err := json.Marshal(mergedPolicy)
		if err != nil {
			return UpdateProjectOutput{}, fmt.Errorf("marshal completion policy failed: %w", err)
		}
		fields = append(fields, fmt.Sprintf("completion_policy = $%d", argPos))
		args = append(args, policyJSON)
		argPos++
	}

	if input.OpenVikingNamespace != nil {
		ns := strings.TrimSpace(*input.OpenVikingNamespace)
		if ns == "" {
			return UpdateProjectOutput{}, NewInvalidInputError(map[string]string{"openvikingNamespace": "cannot be empty"})
		}
		fields = append(fields, fmt.Sprintf("openviking_namespace = $%d", argPos))
		args = append(args, ns)
		argPos++
	}

	if input.OpenVikingWorkspaceID != nil {
		ws := strings.TrimSpace(*input.OpenVikingWorkspaceID)
		fields = append(fields, fmt.Sprintf("openviking_workspace_id = $%d", argPos))
		args = append(args, ws)
		argPos++
	}

	if len(fields) == 0 {
		return UpdateProjectOutput{}, NewInvalidInputError(map[string]string{"body": "no effective fields to update"})
	}

	fields = append(fields, "updated_at = NOW()")
	args = append(args, projectID)

	var output UpdateProjectOutput
	err = s.db.QueryRowContext(ctx, fmt.Sprintf(`
		UPDATE projects
		SET %s
		WHERE id = $%d
		RETURNING id, name, status, updated_at
	`, strings.Join(fields, ", "), argPos), args...).Scan(
		&output.ProjectID,
		&output.Name,
		&output.Status,
		&output.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return UpdateProjectOutput{}, ErrProjectNotFound
	}
	if err != nil {
		return UpdateProjectOutput{}, fmt.Errorf("update project failed: %w", err)
	}

	if output.Name == "" {
		output.Name = updatedName
	}
	if output.Status == "" {
		output.Status = existingStatus
	}

	return output, nil
}

func (s *Service) Complete(ctx context.Context, projectID string, input CompleteProjectInput) (CompleteProjectOutput, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return CompleteProjectOutput{}, NewInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}
	if !input.Confirm {
		return CompleteProjectOutput{}, NewInvalidInputError(map[string]string{"confirm": "must be true"})
	}

	var currentStatus string
	var completionPolicyRaw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT status, completion_policy
		FROM projects
		WHERE id = $1
	`, projectID).Scan(&currentStatus, &completionPolicyRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return CompleteProjectOutput{}, ErrProjectNotFound
	}
	if err != nil {
		return CompleteProjectOutput{}, fmt.Errorf("load project for completion failed: %w", err)
	}
	if currentStatus == projectStatusArchived {
		return CompleteProjectOutput{}, ErrProjectArchived
	}

	policy, err := decodeCompletionPolicy(completionPolicyRaw)
	if err != nil {
		return CompleteProjectOutput{}, fmt.Errorf("decode completion policy failed: %w", err)
	}

	if currentStatus == projectStatusCompleted {
		return CompleteProjectOutput{
			ProjectID: projectID,
			Status:    projectStatusCompleted,
			PolicyResult: CompletionPolicyResult{
				Passed:      true,
				FailedItems: []CompletionPolicyResultItem{},
			},
		}, nil
	}

	var requiredNotDone int
	var blockedCount int
	var failedCount int
	var reviewPendingCount int

	err = s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(COUNT(*) FILTER (WHERE is_required AND status <> 'done'), 0) AS required_not_done,
			COALESCE(COUNT(*) FILTER (WHERE status = 'blocked'), 0) AS blocked_count,
			COALESCE(COUNT(*) FILTER (WHERE status = 'failed'), 0) AS failed_count,
			COALESCE(COUNT(*) FILTER (WHERE status IN ('submitted', 'reviewing')), 0) AS review_pending_count
		FROM tasks
		WHERE project_id = $1
	`, projectID).Scan(&requiredNotDone, &blockedCount, &failedCount, &reviewPendingCount)
	if err != nil {
		return CompleteProjectOutput{}, fmt.Errorf("check completion policy failed: %w", err)
	}

	failedItems := evaluateCompletionPolicy(policy, requiredNotDone, blockedCount, failedCount, reviewPendingCount)
	if len(failedItems) > 0 {
		return CompleteProjectOutput{}, &CompletionPolicyFailedError{failedItems: failedItems}
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE projects
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`, projectID, projectStatusCompleted); err != nil {
		return CompleteProjectOutput{}, fmt.Errorf("update project completion status failed: %w", err)
	}

	if s.onProjectCompleted != nil {
		s.onProjectCompleted(ctx, projectID)
	}

	return CompleteProjectOutput{
		ProjectID: projectID,
		Status:    projectStatusCompleted,
		PolicyResult: CompletionPolicyResult{
			Passed:      true,
			FailedItems: []CompletionPolicyResultItem{},
		},
	}, nil
}

func (s *Service) Archive(ctx context.Context, projectID string, input ArchiveProjectInput) (ArchiveProjectOutput, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ArchiveProjectOutput{}, NewInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}
	if !input.Confirm {
		return ArchiveProjectOutput{}, NewInvalidInputError(map[string]string{"confirm": "must be true"})
	}

	var output ArchiveProjectOutput
	err := s.db.QueryRowContext(ctx, `
		UPDATE projects
		SET status = $2,
			updated_at = NOW()
		WHERE id = $1
		  AND status <> $2
		RETURNING id, status, updated_at
	`, projectID, projectStatusArchived).Scan(&output.ProjectID, &output.Status, &output.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		var status string
		err = s.db.QueryRowContext(ctx, `SELECT status FROM projects WHERE id = $1`, projectID).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return ArchiveProjectOutput{}, ErrProjectNotFound
		}
		if err != nil {
			return ArchiveProjectOutput{}, fmt.Errorf("load project for archive failed: %w", err)
		}
		if strings.TrimSpace(status) == projectStatusArchived {
			var updatedAt time.Time
			if err := s.db.QueryRowContext(ctx, `SELECT updated_at FROM projects WHERE id = $1`, projectID).Scan(&updatedAt); err != nil {
				return ArchiveProjectOutput{}, fmt.Errorf("load archived project timestamp failed: %w", err)
			}
			return ArchiveProjectOutput{
				ProjectID: projectID,
				Status:    projectStatusArchived,
				UpdatedAt: updatedAt,
			}, nil
		}
		return ArchiveProjectOutput{}, fmt.Errorf("archive project failed: missing update result")
	}
	if err != nil {
		return ArchiveProjectOutput{}, fmt.Errorf("archive project failed: %w", err)
	}
	return output, nil
}

func (s *Service) AssertWritable(ctx context.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return NewInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM projects WHERE id = $1`, projectID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("load project status failed: %w", err)
	}
	if strings.TrimSpace(status) == projectStatusArchived {
		return ErrProjectArchived
	}
	return nil
}

func (s *Service) RefreshProgress(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM projects
		ORDER BY updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("list projects for progress refresh failed: %w", err)
	}
	defer rows.Close()

	projectIDs := make([]string, 0)
	for rows.Next() {
		var projectID string
		if err := rows.Scan(&projectID); err != nil {
			return 0, fmt.Errorf("scan project id for progress refresh failed: %w", err)
		}
		projectIDs = append(projectIDs, projectID)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate project ids for progress refresh failed: %w", err)
	}

	updated := 0
	for _, projectID := range projectIDs {
		var doneCount int
		var totalCount int
		var blockedCount int
		if err := s.db.QueryRowContext(ctx, `
			SELECT
				COALESCE(COUNT(*) FILTER (WHERE status = 'done'), 0) AS done_count,
				COALESCE(COUNT(*), 0) AS total_count,
				COALESCE(COUNT(*) FILTER (WHERE status = 'blocked'), 0) AS blocked_count
			FROM tasks
			WHERE project_id = $1
		`, projectID).Scan(&doneCount, &totalCount, &blockedCount); err != nil {
			return updated, fmt.Errorf("calculate project progress failed: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO project_progress_snapshots (
				project_id, done_count, total_count, blocked_count, updated_at
			) VALUES ($1, $2, $3, $4, NOW())
			ON CONFLICT (project_id)
			DO UPDATE SET
				done_count = EXCLUDED.done_count,
				total_count = EXCLUDED.total_count,
				blocked_count = EXCLUDED.blocked_count,
				updated_at = NOW()
		`, projectID, doneCount, totalCount, blockedCount); err != nil {
			return updated, fmt.Errorf("upsert project progress snapshot failed: %w", err)
		}
		updated++
	}
	return updated, nil
}

func (s *Service) FindCompletableProjects(ctx context.Context, limit int) ([]CompletionCandidate, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, completion_policy
		FROM projects
		WHERE status = $1
		ORDER BY updated_at DESC
		LIMIT $2
	`, projectStatusActive, limit)
	if err != nil {
		return nil, fmt.Errorf("list active projects for completion check failed: %w", err)
	}
	defer rows.Close()

	candidates := make([]CompletionCandidate, 0)
	for rows.Next() {
		var projectID string
		var name string
		var policyRaw []byte
		if err := rows.Scan(&projectID, &name, &policyRaw); err != nil {
			return nil, fmt.Errorf("scan project completion candidate failed: %w", err)
		}
		policy, err := decodeCompletionPolicy(policyRaw)
		if err != nil {
			return nil, fmt.Errorf("decode completion policy failed: %w", err)
		}

		var requiredNotDone int
		var blockedCount int
		var failedCount int
		var reviewPendingCount int
		if err := s.db.QueryRowContext(ctx, `
			SELECT
				COALESCE(COUNT(*) FILTER (WHERE is_required AND status <> 'done'), 0) AS required_not_done,
				COALESCE(COUNT(*) FILTER (WHERE status = 'blocked'), 0) AS blocked_count,
				COALESCE(COUNT(*) FILTER (WHERE status = 'failed'), 0) AS failed_count,
				COALESCE(COUNT(*) FILTER (WHERE status IN ('submitted', 'reviewing')), 0) AS review_pending_count
			FROM tasks
			WHERE project_id = $1
		`, projectID).Scan(&requiredNotDone, &blockedCount, &failedCount, &reviewPendingCount); err != nil {
			return nil, fmt.Errorf("calculate completion candidate stats failed: %w", err)
		}
		failedItems := evaluateCompletionPolicy(policy, requiredNotDone, blockedCount, failedCount, reviewPendingCount)
		if len(failedItems) > 0 {
			continue
		}
		candidates = append(candidates, CompletionCandidate{ProjectID: projectID, Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate completion candidates failed: %w", err)
	}

	return candidates, nil
}

func validateCreateInput(input CreateProjectInput) map[string]string {
	details := make(map[string]string)

	if n := utf8.RuneCountInString(input.Name); n < 2 || n > 80 {
		details["name"] = "must be between 2 and 80 characters"
	}
	if n := utf8.RuneCountInString(input.Goal); n < 4 || n > 200 {
		details["goal"] = "must be between 4 and 200 characters"
	}
	if n := utf8.RuneCountInString(input.Description); n > 500 {
		details["description"] = "must be at most 500 characters"
	}

	return details
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

func defaultCompletionPolicy() CompletionPolicy {
	return CompletionPolicy{
		RequiredTasks: "all_required_done",
		BlockedTasks:  "none",
		FailedTasks:   "none",
		ReviewPolicy:  "all_submitted_reviewed",
	}
}

func decodeCompletionPolicy(raw []byte) (CompletionPolicy, error) {
	policy := defaultCompletionPolicy()
	if len(raw) == 0 {
		return policy, nil
	}

	if err := json.Unmarshal(raw, &policy); err != nil {
		return CompletionPolicy{}, err
	}

	if policy.RequiredTasks == "" {
		policy.RequiredTasks = "all_required_done"
	}
	if policy.BlockedTasks == "" {
		policy.BlockedTasks = "none"
	}
	if policy.FailedTasks == "" {
		policy.FailedTasks = "none"
	}
	if policy.ReviewPolicy == "" {
		policy.ReviewPolicy = "all_submitted_reviewed"
	}

	return policy, nil
}

func applyCompletionPolicyPatch(existing CompletionPolicy, patch CompletionPolicyPatch) (CompletionPolicy, map[string]string) {
	merged := existing
	if patch.RequiredTasks != nil {
		merged.RequiredTasks = strings.TrimSpace(*patch.RequiredTasks)
	}
	if patch.BlockedTasks != nil {
		merged.BlockedTasks = strings.TrimSpace(*patch.BlockedTasks)
	}
	if patch.FailedTasks != nil {
		merged.FailedTasks = strings.TrimSpace(*patch.FailedTasks)
	}
	if patch.ReviewPolicy != nil {
		merged.ReviewPolicy = strings.TrimSpace(*patch.ReviewPolicy)
	}

	details := make(map[string]string)
	if merged.RequiredTasks != "all_required_done" && merged.RequiredTasks != "optional" {
		details["completionPolicy.requiredTasks"] = "must be one of: all_required_done, optional"
	}
	if merged.BlockedTasks != "none" && merged.BlockedTasks != "allow" {
		details["completionPolicy.blockedTasks"] = "must be one of: none, allow"
	}
	if merged.FailedTasks != "none" && merged.FailedTasks != "allow" {
		details["completionPolicy.failedTasks"] = "must be one of: none, allow"
	}
	if merged.ReviewPolicy != "" && merged.ReviewPolicy != "all_submitted_reviewed" && merged.ReviewPolicy != "optional" {
		details["completionPolicy.reviewPolicy"] = "must be one of: all_submitted_reviewed, optional"
	}
	if merged.ReviewPolicy == "" {
		merged.ReviewPolicy = "all_submitted_reviewed"
	}

	return merged, details
}

func evaluateCompletionPolicy(
	policy CompletionPolicy,
	requiredNotDone int,
	blockedCount int,
	failedCount int,
	reviewPendingCount int,
) []CompletionPolicyResultItem {
	failedItems := make([]CompletionPolicyResultItem, 0)

	if policy.RequiredTasks == "all_required_done" && requiredNotDone > 0 {
		failedItems = append(failedItems, CompletionPolicyResultItem{
			Code:    "REQUIRED_TASKS_NOT_DONE",
			Message: fmt.Sprintf("%d required tasks are not done", requiredNotDone),
		})
	}
	if policy.BlockedTasks == "none" && blockedCount > 0 {
		failedItems = append(failedItems, CompletionPolicyResultItem{
			Code:    "BLOCKED_TASKS_PRESENT",
			Message: fmt.Sprintf("%d blocked tasks remain", blockedCount),
		})
	}
	if policy.FailedTasks == "none" && failedCount > 0 {
		failedItems = append(failedItems, CompletionPolicyResultItem{
			Code:    "FAILED_TASKS_PRESENT",
			Message: fmt.Sprintf("%d failed tasks remain", failedCount),
		})
	}
	if policy.ReviewPolicy == "all_submitted_reviewed" && reviewPendingCount > 0 {
		failedItems = append(failedItems, CompletionPolicyResultItem{
			Code:    "REVIEW_PENDING",
			Message: fmt.Sprintf("%d submitted/reviewing tasks remain", reviewPendingCount),
		})
	}

	return failedItems
}
