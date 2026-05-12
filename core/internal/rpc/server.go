package rpc

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/redis/go-redis/v9"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
	aitaskv1 "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/rpc/gen/aitask/v1"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/rpc/gen/aitask/v1/aitaskv1connect"
	agentsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/agents"
	contextsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/context"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
	projectsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/projects"
	roomsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/room"
	tasksvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/tasks"
)

type RateLimitOptions struct {
	Enabled         bool
	Store           redis.Cmdable
	Capacity        int
	RefillPerSecond float64
	KeyPrefix       string
}

type ServerDeps struct {
	Agents               *agentsvc.Service
	Tasks                *tasksvc.Service
	Projects             *projectsvc.Service
	Context              *contextsvc.Service
	Room                 *roomsvc.Service
	OpenViking           openviking.MemoryClient
	ConsoleOperatorLabel string
	RateLimit            RateLimitOptions
	Logger               *slog.Logger
}

type identityContextKey struct{}

func NewHandler(deps ServerDeps) http.Handler {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	interceptor := newUnaryServerInterceptor(interceptorOptions{
		Verifier:      deps.Agents,
		OperatorLabel: deps.ConsoleOperatorLabel,
		RateLimit:     deps.RateLimit,
		Logger:        logger,
	})
	opts := []connect.HandlerOption{connect.WithInterceptors(interceptor)}

	mux := http.NewServeMux()

	agentPath, agentHandler := aitaskv1connect.NewAgentServiceHandler(&agentServiceHandler{}, opts...)
	mux.Handle(agentPath, agentHandler)

	bootstrapPath, bootstrapHandler := aitaskv1connect.NewBootstrapServiceHandler(&bootstrapServiceHandler{
		projects:   deps.Projects,
		context:    deps.Context,
		rooms:      deps.Room,
		openViking: deps.OpenViking,
	}, opts...)
	mux.Handle(bootstrapPath, bootstrapHandler)

	contextPath, contextHandler := aitaskv1connect.NewContextServiceHandler(&contextServiceHandler{
		context: deps.Context,
	}, opts...)
	mux.Handle(contextPath, contextHandler)

	taskPath, taskHandler := aitaskv1connect.NewTaskServiceHandler(&taskServiceHandler{
		tasks: deps.Tasks,
	}, opts...)
	mux.Handle(taskPath, taskHandler)

	return mux
}

type interceptorOptions struct {
	Verifier      *agentsvc.Service
	OperatorLabel string
	RateLimit     RateLimitOptions
	Logger        *slog.Logger
}

func newUnaryServerInterceptor(opts interceptorOptions) connect.Interceptor {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	operatorLabel := strings.TrimSpace(opts.OperatorLabel)
	if operatorLabel == "" {
		operatorLabel = "local-operator"
	}
	keyPrefix := strings.TrimSpace(opts.RateLimit.KeyPrefix)
	if keyPrefix == "" {
		keyPrefix = "rl:token"
	}

	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			actor, tokenID, err := resolveIdentity(ctx, req.Header(), opts.Verifier, operatorLabel)
			if err != nil {
				return nil, err
			}

			if opts.RateLimit.Enabled && opts.RateLimit.Store != nil && opts.RateLimit.Capacity > 0 && opts.RateLimit.RefillPerSecond > 0 {
				identityKey := tokenID
				if strings.TrimSpace(identityKey) == "" {
					identityKey = actorIdentityKey(actor)
				}
				allowed, _, rateErr := allowTokenBucket(ctx, opts.RateLimit.Store, keyPrefix, identityKey, opts.RateLimit.Capacity, opts.RateLimit.RefillPerSecond)
				if rateErr == nil && !allowed {
					return nil, appConnectError("RATE_LIMITED", connect.CodeResourceExhausted, "Too many requests", true)
				}
			}

			ctx = context.WithValue(ctx, identityContextKey{}, actor)
			start := time.Now()
			resp, err := next(ctx, req)
			if err != nil {
				connectErr := ensureAppMetadata(err)
				logger.Warn("rpc request failed", "procedure", req.Spec().Procedure, "duration", time.Since(start).String(), "error", connectErr)
				return nil, connectErr
			}
			logger.Debug("rpc request finished", "procedure", req.Spec().Procedure, "duration", time.Since(start).String())
			return resp, nil
		}
	})
}

func resolveIdentity(ctx context.Context, header http.Header, verifier *agentsvc.Service, operatorLabel string) (identity.Identity, string, error) {
	raw := strings.TrimSpace(header.Get("Authorization"))
	if raw == "" {
		return identity.Operator(operatorLabel), "", nil
	}
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return identity.Identity{}, "", appConnectError("AGENT_TOKEN_INVALID", connect.CodeUnauthenticated, "Agent token is invalid", false)
	}
	token := strings.TrimSpace(parts[1])
	if token == "" || verifier == nil {
		return identity.Identity{}, "", appConnectError("AGENT_TOKEN_INVALID", connect.CodeUnauthenticated, "Agent token is invalid", false)
	}

	agentIdentity, err := verifier.VerifyToken(ctx, token)
	if err != nil {
		if errors.Is(err, agentsvc.ErrAgentTokenExpired) {
			return identity.Identity{}, "", appConnectError("AGENT_TOKEN_EXPIRED", connect.CodeUnauthenticated, "Agent token expired", false)
		}
		return identity.Identity{}, "", appConnectError("AGENT_TOKEN_INVALID", connect.CodeUnauthenticated, "Agent token is invalid", false)
	}
	return identity.Identity{SenderType: identity.SenderTypeAgent, Agent: agentIdentity}, token, nil
}

func actorIdentityKey(actor identity.Identity) string {
	if actor.IsAgent() {
		return "agent:" + strings.TrimSpace(actor.Agent.AgentID)
	}
	if actor.IsOperator() {
		return "operator:" + strings.TrimSpace(actor.OperatorLabel)
	}
	return "system"
}

var rpcTokenBucketScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local refill_per_sec = tonumber(ARGV[2])
local capacity = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])
local ttl_ms = tonumber(ARGV[5])

local values = redis.call("HMGET", KEYS[1], "tokens", "ts")
local tokens = tonumber(values[1])
local ts = tonumber(values[2])
if tokens == nil then tokens = capacity end
if ts == nil then ts = now end

local delta = now - ts
if delta < 0 then delta = 0 end
tokens = math.min(capacity, tokens + (delta * refill_per_sec / 1000))

local allowed = 0
if tokens >= requested then
  allowed = 1
  tokens = tokens - requested
end

redis.call("HMSET", KEYS[1], "tokens", tokens, "ts", now)
redis.call("PEXPIRE", KEYS[1], ttl_ms)
return {allowed, tokens}
`)

func allowTokenBucket(ctx context.Context, store redis.Cmdable, keyPrefix string, identityKey string, capacity int, refillPerSecond float64) (bool, int, error) {
	hash := sha256.Sum256([]byte(identityKey))
	key := keyPrefix + ":" + hex.EncodeToString(hash[:16])
	nowMillis := time.Now().UTC().UnixMilli()
	ttlMillis := int64(math.Ceil((float64(capacity)/refillPerSecond)*2000 + 1000))

	rawResult, err := rpcTokenBucketScript.Run(
		ctx,
		store,
		[]string{key},
		nowMillis,
		refillPerSecond,
		capacity,
		1,
		ttlMillis,
	).Result()
	if err != nil {
		return true, 0, err
	}
	items, ok := rawResult.([]any)
	if !ok || len(items) < 2 {
		return true, 0, nil
	}
	allowed := true
	if v, ok := items[0].(int64); ok {
		allowed = v == 1
	}
	remaining := 0
	switch value := items[1].(type) {
	case int64:
		remaining = int(value)
	case float64:
		remaining = int(value)
	}
	return allowed, remaining, nil
}

func appConnectError(code string, connectCode connect.Code, message string, retriable bool) error {
	err := connect.NewError(connectCode, errors.New(message))
	err.Meta().Set("x-aitask-code", strings.TrimSpace(code))
	err.Meta().Set("x-aitask-retriable", strconv.FormatBool(retriable))
	return err
}

func ensureAppMetadata(err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if strings.TrimSpace(connectErr.Meta().Get("x-aitask-code")) == "" {
			connectErr.Meta().Set("x-aitask-code", "INTERNAL")
		}
		if strings.TrimSpace(connectErr.Meta().Get("x-aitask-retriable")) == "" {
			connectErr.Meta().Set("x-aitask-retriable", strconv.FormatBool(connect.CodeOf(connectErr) == connect.CodeUnavailable))
		}
		return connectErr
	}
	return appConnectError("INTERNAL", connect.CodeInternal, "Internal server error", true)
}

func identityFromContext(ctx context.Context) identity.Identity {
	raw := ctx.Value(identityContextKey{})
	value, ok := raw.(identity.Identity)
	if !ok {
		return identity.Operator("local-operator")
	}
	return value
}

type agentServiceHandler struct{}

func (h *agentServiceHandler) WhoAmI(ctx context.Context, _ *connect.Request[aitaskv1.WhoAmIRequest]) (*connect.Response[aitaskv1.WhoAmIResponse], error) {
	actor := identityFromContext(ctx)
	if !actor.IsAgent() {
		return nil, appConnectError("PROJECT_ACCESS_DENIED", connect.CodePermissionDenied, "Agent identity required", false)
	}
	return connect.NewResponse(&aitaskv1.WhoAmIResponse{
		Identity: &aitaskv1.AgentIdentity{
			AgentId:         actor.Agent.AgentID,
			AgentType:       actor.Agent.AgentType,
			Role:            actor.Agent.Role,
			Scopes:          actor.Agent.Scopes,
			AllowedProjects: actor.Agent.AllowedProjects,
		},
	}), nil
}

type taskServiceHandler struct {
	tasks *tasksvc.Service
}

func (h *taskServiceHandler) GetCurrentTask(ctx context.Context, req *connect.Request[aitaskv1.GetCurrentTaskRequest]) (*connect.Response[aitaskv1.GetCurrentTaskResponse], error) {
	actor := identityFromContext(ctx)
	if !actor.IsAgent() {
		return nil, appConnectError("PROJECT_ACCESS_DENIED", connect.CodePermissionDenied, "Agent identity required", false)
	}
	projectID := strings.TrimSpace(req.Msg.GetProjectId())
	if projectID == "" {
		return nil, appConnectError("INVALID_ARGUMENT", connect.CodeInvalidArgument, "project_id cannot be empty", false)
	}
	if !actor.Agent.CanAccessProject(projectID) {
		return nil, appConnectError("PROJECT_ACCESS_DENIED", connect.CodePermissionDenied, "No access to this project", false)
	}
	if h.tasks == nil {
		return nil, appConnectError("TASK_SERVICE_UNAVAILABLE", connect.CodeUnavailable, "Task service unavailable", true)
	}

	items, err := h.tasks.List(ctx, projectID, tasksvc.TaskFilters{
		Status:          "running",
		AssigneeAgentID: actor.Agent.AgentID,
	})
	if err != nil {
		return nil, mapTaskError(err)
	}
	if len(items) == 0 {
		items, err = h.tasks.List(ctx, projectID, tasksvc.TaskFilters{
			Status:          "delegated",
			AssigneeAgentID: actor.Agent.AgentID,
		})
		if err != nil {
			return nil, mapTaskError(err)
		}
	}

	if len(items) == 0 {
		return connect.NewResponse(&aitaskv1.GetCurrentTaskResponse{
			NextAction: &aitaskv1.NextAction{
				Type:    "wait_task",
				Message: "No delegated task currently assigned",
				Command: "aitask room watch",
			},
		}), nil
	}

	current := items[0]
	next := &aitaskv1.NextAction{
		Type:    "continue",
		Message: "Continue task execution",
	}
	if current.Status == tasksvc.StatusDelegated {
		next = &aitaskv1.NextAction{
			Type:    "start_task",
			Message: "Start delegated task",
			Command: "aitask task start " + current.TaskID,
		}
	}
	return connect.NewResponse(&aitaskv1.GetCurrentTaskResponse{
		Task:       mapTaskRecord(current),
		NextAction: next,
	}), nil
}

func (h *taskServiceHandler) StartTask(ctx context.Context, req *connect.Request[aitaskv1.StartTaskRequest]) (*connect.Response[aitaskv1.StartTaskResponse], error) {
	actor := identityFromContext(ctx)
	if h.tasks == nil {
		return nil, appConnectError("TASK_SERVICE_UNAVAILABLE", connect.CodeUnavailable, "Task service unavailable", true)
	}
	item, err := h.tasks.Start(ctx, actor, strings.TrimSpace(req.Msg.GetProjectId()), strings.TrimSpace(req.Msg.GetTaskId()), tasksvc.StartTaskInput{
		RunID: strings.TrimSpace(req.Msg.GetRunId()),
	})
	if err != nil {
		return nil, mapTaskError(err)
	}
	response := &aitaskv1.StartTaskResponse{
		TaskId:      item.TaskID,
		Status:      string(item.Status),
		ActiveRunId: strings.TrimSpace(req.Msg.GetRunId()),
	}
	if item.UpdatedAt.After(time.Time{}) {
		response.StartedAt = item.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return connect.NewResponse(response), nil
}

func (h *taskServiceHandler) SubmitTask(ctx context.Context, req *connect.Request[aitaskv1.SubmitTaskRequest]) (*connect.Response[aitaskv1.SubmitTaskResponse], error) {
	actor := identityFromContext(ctx)
	if h.tasks == nil {
		return nil, appConnectError("TASK_SERVICE_UNAVAILABLE", connect.CodeUnavailable, "Task service unavailable", true)
	}
	artifacts := make([]tasksvc.SubmitArtifactInput, 0, len(req.Msg.GetArtifacts()))
	for _, item := range req.Msg.GetArtifacts() {
		artifacts = append(artifacts, tasksvc.SubmitArtifactInput{
			ArtifactType: item.GetArtifactType(),
			URI:          item.GetUri(),
			Name:         item.GetName(),
		})
	}
	item, err := h.tasks.Submit(ctx, actor, strings.TrimSpace(req.Msg.GetProjectId()), strings.TrimSpace(req.Msg.GetTaskId()), tasksvc.SubmitTaskInput{
		RunID:          strings.TrimSpace(req.Msg.GetRunId()),
		ResultMarkdown: req.Msg.GetResultMarkdown(),
		Artifacts:      artifacts,
	})
	if err != nil {
		return nil, mapTaskError(err)
	}
	return connect.NewResponse(&aitaskv1.SubmitTaskResponse{
		TaskId: item.TaskID,
		Status: string(item.Status),
		NextAction: &aitaskv1.NextAction{
			Type:    "review_pending",
			Message: "Task submitted, waiting for review",
			Command: "aitask room watch",
		},
	}), nil
}

type contextServiceHandler struct {
	context *contextsvc.Service
}

func (h *contextServiceHandler) Report(ctx context.Context, req *connect.Request[aitaskv1.ReportRequest]) (*connect.Response[aitaskv1.ReportResponse], error) {
	actor := identityFromContext(ctx)
	if h.context == nil {
		return nil, appConnectError("INTERNAL", connect.CodeUnavailable, "Context service unavailable", true)
	}
	output, err := h.context.Report(ctx, actor, contextsvc.ReportInput{
		ProjectID:            strings.TrimSpace(req.Msg.GetProjectId()),
		RunID:                strings.TrimSpace(req.Msg.GetRunId()),
		ReportedInputTokens:  int(req.Msg.GetReportedInputTokens()),
		ReportedOutputTokens: int(req.Msg.GetReportedOutputTokens()),
		MaxContextTokens:     int(req.Msg.GetMaxContextTokens()),
		Source:               strings.TrimSpace(req.Msg.GetSource()),
	})
	if err != nil {
		return nil, mapContextError(err)
	}
	return connect.NewResponse(&aitaskv1.ReportResponse{
		Budget: &aitaskv1.ContextBudget{
			MaxContextTokens:    int32(output.Budget.MaxContextTokens),
			EstimatedUsedTokens: int32(output.Budget.EstimatedUsedTokens),
			State:               output.Budget.State,
			UsageRatio:          output.Budget.UsageRatio,
		},
		Warnings: output.Warnings,
		NextAction: &aitaskv1.NextAction{
			Type:    output.NextAction.Type,
			Message: output.NextAction.Message,
			Command: output.NextAction.Command,
		},
	}), nil
}

func (h *contextServiceHandler) CreateHandoff(ctx context.Context, req *connect.Request[aitaskv1.CreateHandoffRequest]) (*connect.Response[aitaskv1.CreateHandoffResponse], error) {
	actor := identityFromContext(ctx)
	if h.context == nil {
		return nil, appConnectError("INTERNAL", connect.CodeUnavailable, "Context service unavailable", true)
	}
	output, err := h.context.CreateHandoff(ctx, actor, contextsvc.CreateHandoffInput{
		ProjectID:       strings.TrimSpace(req.Msg.GetProjectId()),
		TaskID:          strings.TrimSpace(req.Msg.GetTaskId()),
		Reason:          strings.TrimSpace(req.Msg.GetReason()),
		HandoffMarkdown: req.Msg.GetHandoffMarkdown(),
	})
	if err != nil {
		return nil, mapContextError(err)
	}
	return connect.NewResponse(&aitaskv1.CreateHandoffResponse{
		HandoffId:     output.HandoffID,
		OpenvikingUri: output.OpenVikingURI,
		NextAction: &aitaskv1.NextAction{
			Type:    output.NextAction.Type,
			Message: output.NextAction.Message,
			Command: output.NextAction.Command,
		},
	}), nil
}

func (h *contextServiceHandler) GetCurrentHandoff(ctx context.Context, req *connect.Request[aitaskv1.GetCurrentHandoffRequest]) (*connect.Response[aitaskv1.GetCurrentHandoffResponse], error) {
	actor := identityFromContext(ctx)
	if h.context == nil {
		return nil, appConnectError("INTERNAL", connect.CodeUnavailable, "Context service unavailable", true)
	}
	output, err := h.context.GetCurrentHandoff(ctx, actor, strings.TrimSpace(req.Msg.GetProjectId()))
	if err != nil {
		return nil, mapContextError(err)
	}
	refs := make([]*aitaskv1.ContextRef, 0, len(output.ContextRefs))
	for _, ref := range output.ContextRefs {
		refs = append(refs, &aitaskv1.ContextRef{
			Uri:             ref.URI,
			Title:           ref.Title,
			EstimatedTokens: int32(ref.EstimatedTokens),
		})
	}
	return connect.NewResponse(&aitaskv1.GetCurrentHandoffResponse{
		HandoffId:       output.HandoffID,
		TaskId:          output.TaskID,
		Summary:         output.Summary,
		HandoffMarkdown: output.HandoffMarkdown,
		ContextRefs:     refs,
	}), nil
}

type bootstrapServiceHandler struct {
	projects   *projectsvc.Service
	context    *contextsvc.Service
	rooms      *roomsvc.Service
	openViking openviking.MemoryClient
}

func (h *bootstrapServiceHandler) Bootstrap(ctx context.Context, req *connect.Request[aitaskv1.BootstrapRequest]) (*connect.Response[aitaskv1.BootstrapResponse], error) {
	projectID := strings.TrimSpace(req.Msg.GetProjectId())
	if projectID == "" {
		return nil, appConnectError("INVALID_ARGUMENT", connect.CodeInvalidArgument, "project_id cannot be empty", false)
	}
	if h.projects == nil || h.context == nil {
		return nil, appConnectError("INTERNAL", connect.CodeUnavailable, "Bootstrap service unavailable", true)
	}

	actor := identityFromContext(ctx)
	if actor.IsAgent() && !actor.Agent.CanAccessProject(projectID) {
		return nil, appConnectError("PROJECT_ACCESS_DENIED", connect.CodePermissionDenied, "No access to this project", false)
	}

	project, err := h.projects.Get(ctx, projectID)
	if err != nil {
		return nil, mapProjectError(err)
	}

	runID, budget, err := h.context.FindActiveRunBudget(ctx, actor, projectID)
	if err != nil {
		return nil, mapContextError(err)
	}

	contextRefs := make([]*aitaskv1.ContextRef, 0)
	if h.openViking != nil {
		if items, listErr := h.openViking.List(ctx, projectID); listErr == nil {
			for i, item := range items {
				if i >= 80 {
					break
				}
				contextRefs = append(contextRefs, &aitaskv1.ContextRef{
					Uri:             item.URI,
					Title:           item.Title,
					EstimatedTokens: 300,
				})
			}
		}
	}

	roomID := project.RoomID
	if roomID == "" && h.rooms != nil {
		if snapshot, roomErr := h.rooms.GetRoom(ctx, projectID); roomErr == nil {
			roomID = snapshot.RoomID
		}
	}
	unreadMentions := 0
	if h.rooms != nil {
		if count, mentionErr := h.rooms.UnreadMentions(ctx, actor, projectID); mentionErr == nil {
			unreadMentions = count
		}
	}

	nextAction := &aitaskv1.NextAction{
		Type:    "read_delegated_task",
		Message: "Run aitask task current",
		Command: "aitask task current",
	}
	if pending, pendingErr := h.context.HasPendingHandoff(ctx, actor, projectID); pendingErr == nil && pending {
		nextAction = &aitaskv1.NextAction{
			Type:    "handoff_current",
			Message: "Run aitask context handoff current",
			Command: "aitask context handoff current",
		}
	}

	identityMsg := &aitaskv1.AgentIdentity{}
	if actor.IsAgent() {
		identityMsg = &aitaskv1.AgentIdentity{
			AgentId:         actor.Agent.AgentID,
			AgentType:       actor.Agent.AgentType,
			Role:            actor.Agent.Role,
			Scopes:          actor.Agent.Scopes,
			AllowedProjects: actor.Agent.AllowedProjects,
		}
	}

	return connect.NewResponse(&aitaskv1.BootstrapResponse{
		Identity: identityMsg,
		Project: &aitaskv1.ProjectRef{
			ProjectId: project.ProjectID,
			Name:      project.Name,
			Status:    project.Status,
		},
		Session: &aitaskv1.SessionRef{
			SessionId: project.ActiveSessionID,
			Status:    "active",
		},
		Run: &aitaskv1.AgentRun{
			RunId: runID,
			ContextBudget: &aitaskv1.ContextBudget{
				MaxContextTokens:    int32(budget.MaxContextTokens),
				EstimatedUsedTokens: int32(budget.EstimatedUsedTokens),
				State:               budget.State,
				UsageRatio:          budget.UsageRatio,
			},
		},
		ContextRefs: contextRefs,
		Room: &aitaskv1.RoomSnapshot{
			RoomId:         roomID,
			RecentSummary:  "",
			UnreadMentions: int32(unreadMentions),
		},
		NextAction: nextAction,
	}), nil
}

func mapTaskRecord(item tasksvc.TaskRecord) *aitaskv1.Task {
	skills := make([]*aitaskv1.SkillRef, 0, len(item.RequiredSkills))
	for _, skill := range item.RequiredSkills {
		skills = append(skills, &aitaskv1.SkillRef{Name: skill})
	}
	delegation := &aitaskv1.TaskDelegation{
		DelegatedByType: defaultDelegatedByType(item.DelegatedByType),
	}
	if item.DelegatedByOperatorLabel != nil {
		delegation.DelegatedByOperatorLabel = strings.TrimSpace(*item.DelegatedByOperatorLabel)
	}
	if item.DelegatedByAgentID != nil {
		delegation.DelegatedByAgentId = strings.TrimSpace(*item.DelegatedByAgentID)
	}
	if item.DelegatedAt != nil {
		delegation.DelegatedAt = item.DelegatedAt.UTC().Format(time.RFC3339)
	}

	task := &aitaskv1.Task{
		TaskId:         item.TaskID,
		ProjectId:      item.ProjectID,
		Title:          item.Title,
		Status:         string(item.Status),
		RequiredSkills: skills,
		RequiredModel:  valueOrEmpty(item.RequiredModel),
		OutputContract: valueOrEmpty(item.OutputContract),
		Priority:       int32(item.Priority),
		Delegation:     delegation,
		Description:    item.Description,
		Goal:           valueOrEmpty(item.Goal),
		Inputs:         valueOrEmpty(item.Inputs),
		Constraints:    valueOrEmpty(item.Constraints),
	}
	if item.AssigneeAgentID != nil {
		task.AssigneeAgentId = strings.TrimSpace(*item.AssigneeAgentID)
	}
	if item.AssigneeAgentType != nil {
		task.AssigneeAgentType = strings.TrimSpace(*item.AssigneeAgentType)
	}
	if item.ActiveRunID != nil {
		task.ActiveRunId = strings.TrimSpace(*item.ActiveRunID)
	}
	if item.LastHeartbeatAt != nil {
		task.LastHeartbeatAt = item.LastHeartbeatAt.UTC().Format(time.RFC3339)
	}
	return task
}

func mapTaskError(err error) error {
	var invalid *tasksvc.InvalidInputError
	if errors.As(err, &invalid) {
		return appConnectError("INVALID_ARGUMENT", connect.CodeInvalidArgument, "Invalid task input", false)
	}
	if errors.Is(err, tasksvc.ErrTaskNotFound) {
		return appConnectError("TASK_NOT_FOUND", connect.CodeNotFound, "Task not found", false)
	}
	if errors.Is(err, tasksvc.ErrTaskNotEligibleForAgent) {
		return appConnectError("TASK_NOT_ELIGIBLE_FOR_AGENT", connect.CodeFailedPrecondition, "Task is not eligible for target agent", false)
	}
	if errors.Is(err, tasksvc.ErrAgentNotBoundToProject) {
		return appConnectError("AGENT_NOT_BOUND_TO_PROJECT", connect.CodeFailedPrecondition, "Agent is not bound to this project", false)
	}
	if errors.Is(err, tasksvc.ErrTaskAlreadyDelegated) {
		return appConnectError("TASK_ALREADY_DELEGATED", connect.CodeFailedPrecondition, "Task already delegated", false)
	}
	if errors.Is(err, tasksvc.ErrTaskNotAssignedToCurrentAgent) {
		return appConnectError("TASK_NOT_ASSIGNED_TO_CURRENT_AGENT", connect.CodePermissionDenied, "Task is not assigned to current agent", false)
	}
	if errors.Is(err, tasksvc.ErrTaskActiveRunMismatch) {
		return appConnectError("TASK_ACTIVE_RUN_MISMATCH", connect.CodeFailedPrecondition, "Task active run mismatch", false)
	}
	if errors.Is(err, tasksvc.ErrTaskDependencyNotDone) {
		return appConnectError("TASK_DEPENDENCY_NOT_DONE", connect.CodeFailedPrecondition, "Task dependency not done", false)
	}
	if errors.Is(err, tasksvc.ErrTaskStatusInvalid) {
		return appConnectError("TASK_STATUS_INVALID", connect.CodeFailedPrecondition, "Task status invalid", false)
	}
	if errors.Is(err, tasksvc.ErrHandoffNotFound) {
		return appConnectError("HANDOFF_NOT_FOUND", connect.CodeNotFound, "Handoff not found", false)
	}
	if errors.Is(err, tasksvc.ErrHandoffAlreadyConsumed) {
		return appConnectError("HANDOFF_ALREADY_CONSUMED", connect.CodeFailedPrecondition, "Handoff already consumed", false)
	}
	if errors.Is(err, tasksvc.ErrProjectAccessDenied) {
		return appConnectError("PROJECT_ACCESS_DENIED", connect.CodePermissionDenied, "No access to project", false)
	}
	if errors.Is(err, tasksvc.ErrReviewScopeRequired) {
		return appConnectError("TASK_NOT_ELIGIBLE_FOR_AGENT", connect.CodePermissionDenied, "task:review scope required", false)
	}
	if errors.Is(err, tasksvc.ErrContextHandoffRequired) {
		return appConnectError("CONTEXT_HANDOFF_REQUIRED", connect.CodeFailedPrecondition, "Context handoff required", false)
	}
	return appConnectError("INTERNAL", connect.CodeInternal, "Internal server error", true)
}

func mapContextError(err error) error {
	if _, ok := contextsvc.IsInvalidInput(err); ok {
		return appConnectError("INVALID_ARGUMENT", connect.CodeInvalidArgument, "Invalid context request", false)
	}
	if errors.Is(err, contextsvc.ErrContextRunAccessDenied) {
		return appConnectError("PROJECT_ACCESS_DENIED", connect.CodePermissionDenied, "No access to project context", false)
	}
	if errors.Is(err, contextsvc.ErrContextRunNotFound) {
		return appConnectError("TASK_ACTIVE_RUN_MISMATCH", connect.CodeNotFound, "Active run not found", false)
	}
	if errors.Is(err, contextsvc.ErrContextHandoffNotFound) {
		return appConnectError("HANDOFF_NOT_FOUND", connect.CodeNotFound, "Handoff not found", false)
	}
	if errors.Is(err, contextsvc.ErrContextHandoffRequired) {
		return appConnectError("CONTEXT_HANDOFF_REQUIRED", connect.CodeFailedPrecondition, "Context handoff required", false)
	}
	if errors.Is(err, contextsvc.ErrContextBudgetExceeded) {
		return appConnectError("CONTEXT_BUDGET_EXCEEDED", connect.CodeFailedPrecondition, "Context budget exceeded", false)
	}
	return appConnectError("INTERNAL", connect.CodeInternal, "Internal server error", true)
}

func mapProjectError(err error) error {
	var invalid *projectsvc.InvalidInputError
	if errors.As(err, &invalid) {
		return appConnectError("INVALID_ARGUMENT", connect.CodeInvalidArgument, "Invalid project input", false)
	}
	if errors.Is(err, projectsvc.ErrProjectNotFound) {
		return appConnectError("PROJECT_NOT_FOUND", connect.CodeNotFound, "Project not found", false)
	}
	if errors.Is(err, projectsvc.ErrProjectCompletionFailed) {
		return appConnectError("PROJECT_COMPLETION_POLICY_FAILED", connect.CodeFailedPrecondition, "Project completion policy check failed", false)
	}
	return appConnectError("INTERNAL", connect.CodeInternal, "Internal server error", true)
}

func defaultDelegatedByType(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "system"
	}
	return trimmed
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

var _ = sql.ErrNoRows
