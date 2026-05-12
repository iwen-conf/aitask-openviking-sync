package codes

const (
	ProjectNotFound             = "PROJECT_NOT_FOUND"
	ProjectAccessDenied         = "PROJECT_ACCESS_DENIED"
	AgentNotBoundToProject      = "AGENT_NOT_BOUND_TO_PROJECT"
	AgentTokenInvalid           = "AGENT_TOKEN_INVALID"
	AgentTokenExpired           = "AGENT_TOKEN_EXPIRED"
	TaskNotFound                = "TASK_NOT_FOUND"
	TaskNotEligibleForAgent     = "TASK_NOT_ELIGIBLE_FOR_AGENT"
	TaskAlreadyDelegated        = "TASK_ALREADY_DELEGATED"
	TaskNotAssignedCurrentAgent = "TASK_NOT_ASSIGNED_TO_CURRENT_AGENT"
	TaskActiveRunMismatch       = "TASK_ACTIVE_RUN_MISMATCH"
	TaskDependencyNotDone       = "TASK_DEPENDENCY_NOT_DONE"
	TaskStatusInvalid           = "TASK_STATUS_INVALID"
	RoomNotFound                = "ROOM_NOT_FOUND"
	RoomAccessDenied            = "ROOM_ACCESS_DENIED"
	RoomMessageTooLarge         = "ROOM_MESSAGE_TOO_LARGE"
	ContextBudgetExceeded       = "CONTEXT_BUDGET_EXCEEDED"
	ContextHandoffRequired      = "CONTEXT_HANDOFF_REQUIRED"
	HandoffNotFound             = "HANDOFF_NOT_FOUND"
	HandoffAlreadyConsumed      = "HANDOFF_ALREADY_CONSUMED"
	OpenVikingWriteFailed       = "OPENVIKING_WRITE_FAILED"
	OpenVikingReadFailed        = "OPENVIKING_READ_FAILED"
)

var Standard21 = []string{
	ProjectNotFound,
	ProjectAccessDenied,
	AgentNotBoundToProject,
	AgentTokenInvalid,
	AgentTokenExpired,
	TaskNotFound,
	TaskNotEligibleForAgent,
	TaskAlreadyDelegated,
	TaskNotAssignedCurrentAgent,
	TaskActiveRunMismatch,
	TaskDependencyNotDone,
	TaskStatusInvalid,
	RoomNotFound,
	RoomAccessDenied,
	RoomMessageTooLarge,
	ContextBudgetExceeded,
	ContextHandoffRequired,
	HandoffNotFound,
	HandoffAlreadyConsumed,
	OpenVikingWriteFailed,
	OpenVikingReadFailed,
}
