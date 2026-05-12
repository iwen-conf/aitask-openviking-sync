/**
 * docs/API/ 唯一契约镜像。
 * 任何字段调整必须先动 docs/API，再同步本文件。
 */

// ───────────────────────────── 通用 ─────────────────────────────

export type Iso8601 = string

export interface ApiErrorEnvelope {
  code: string
  message: string
  retriable: boolean
  details?: Record<string, unknown>
}

export interface CursorList<T> {
  items: T[]
  nextCursor: string | null
}

// ───────────────────────────── Project ─────────────────────────────

export type ProjectStatus =
  | 'draft'
  | 'active'
  | 'paused'
  | 'blocked'
  | 'reviewing'
  | 'completed'
  | 'archived'

export interface ProjectProgress {
  done: number
  total: number
  blocked: number
}

export interface CompletionPolicy {
  requiredTasks: 'all_required_done' | 'optional'
  blockedTasks: 'none' | 'allow'
  failedTasks: 'none' | 'allow'
  reviewPolicy?: 'all_submitted_reviewed' | 'optional'
}

export interface ProjectSummary {
  projectId: string
  name: string
  status: ProjectStatus
  progress: ProjectProgress
  updatedAt: Iso8601
}

export interface Project {
  projectId: string
  name: string
  goal: string
  description: string
  status: ProjectStatus
  activeSessionId: string
  openvikingRoot: string
  openvikingNamespace: string
  openvikingWorkspaceId: string
  roomId: string
  operatorLabel: string
  completionPolicy: CompletionPolicy
  createdAt: Iso8601
  updatedAt: Iso8601
}

export interface CreateProjectInput {
  name: string
  goal: string
  description?: string
}

export interface CreateProjectResponse {
  projectId: string
  name: string
  status: ProjectStatus
  activeSessionId: string
  openvikingRoot: string
  roomId: string
  initCommand: string
}

export interface UpdateProjectInput {
  name?: string
  description?: string
  goal?: string
  completionPolicy?: CompletionPolicy
  openvikingNamespace?: string
  openvikingWorkspaceId?: string
}

export interface CompletionPolicyResultItem {
  code: string
  message: string
  taskId?: string
}

export interface CompleteProjectResponse {
  projectId: string
  status: ProjectStatus
  policyResult: {
    passed: boolean
    failedItems: CompletionPolicyResultItem[]
  }
}

export interface ArchiveProjectResponse {
  projectId: string
  status: ProjectStatus
  updatedAt: Iso8601
}

// ───────────────────────────── Agent ─────────────────────────────

export type AgentType = 'claude-code' | 'codex' | 'gemini' | 'system'
export type AgentRole = 'coordinator' | 'worker' | 'reviewer' | 'observer' | 'system'

export interface Agent {
  agentId: string
  name: string
  agentType: AgentType
  role: AgentRole
  status: 'active' | 'inactive'
  scopes: string[]
  defaultModel?: string
  models?: string[]
  skills?: string[]
  boundProjects?: string[]
  contextState?: 'idle' | 'working' | 'paused'
  lastSeenAt?: Iso8601
  currentTaskId?: string | null
  tokens?: AgentTokenSummary[]
}

export interface AgentTokenSummary {
  tokenId: string
  expiresAt: Iso8601 | null
  revokedAt?: Iso8601 | null
  scopes: string[]
}

export interface CreateAgentInput {
  name: string
  agentType: AgentType
  role: AgentRole
  defaultModel?: string
  skills?: string[]
  models?: string[]
}

export interface RevokeTokenInput {
  reason: string
}

export interface RevokeTokenResponse {
  tokenId: string
  revokedAt: Iso8601
}

export interface IssueTokenInput {
  expiresAt?: Iso8601 | null
  scopes: string[]
}

export interface IssueTokenResponse {
  tokenId: string
  agentToken: string
  expiresAt: Iso8601 | null
}

// ───────────────────────────── Task ─────────────────────────────

export type TaskStatus =
  | 'planned'
  | 'delegated'
  | 'running'
  | 'submitted'
  | 'reviewing'
  | 'done'
  | 'blocked'
  | 'failed'
  | 'cancelled'

export interface Task {
  taskId: string
  projectId: string
  title: string
  description?: string
  goal?: string
  inputs?: string
  constraints?: string
  status: TaskStatus
  parentTaskId?: string | null
  dependencies?: string[]
  assigneeAgentId?: string | null
  assigneeAgentType?: AgentType | null
  delegatedByType: 'operator' | 'agent' | 'system'
  delegatedByOperatorLabel?: string
  delegatedByAgentId?: string
  delegatedAt?: Iso8601
  activeRunId?: string | null
  lastHeartbeatAt?: Iso8601 | null
  requiredSkills?: string[]
  requiredModel?: string
  outputContract?: string
  priority?: number
  createdAt: Iso8601
  updatedAt: Iso8601
}

export interface TaskEvent {
  eventId: string
  projectId: string
  sessionId: string
  taskId?: string | null
  eventType: string
  fromStatus?: TaskStatus | null
  toStatus?: TaskStatus | null
  actorType: 'operator' | 'agent' | 'system'
  actorOperatorLabel?: string | null
  actorAgentId?: string | null
  actorRunId?: string | null
  payload?: Record<string, unknown>
  createdAt: Iso8601
}

export interface CreateTaskInput {
  title: string
  description?: string
  goal?: string
  inputs?: string
  constraints?: string
  parentTaskId?: string
  dependencies?: string[]
  delegateToAgentId?: string
  delegateToAgentType?: AgentType
  requiredSkills?: string[]
  requiredModel?: string
  priority?: number
  outputContract?: string
}

export interface ReviewTaskInput {
  approve: boolean
  reason: string
}

export interface FailTaskInput {
  runId?: string
  reason: string
}

// ───────────────────────────── Room ─────────────────────────────

export type MessageType =
  | 'text'
  | 'task_reference'
  | 'task_status'
  | 'question'
  | 'answer'
  | 'blocker'
  | 'review_request'
  | 'review_result'
  | 'artifact_reference'
  | 'system_event'
  | 'memory_note'
  | 'context_handoff'
  | 'command_request'

export type MessageSenderType = 'agent' | 'operator' | 'system'

export interface MessageSender {
  type: MessageSenderType
  agentId?: string
  agentType?: AgentType
  operatorLabel?: string
}

export interface RoomMessage {
  messageId: string
  projectId: string
  roomId: string
  sender: MessageSender
  messageType: MessageType
  content: string
  payload?: Record<string, unknown>
  createdAt: Iso8601
}

export interface RoomMention {
  mentionId: string
  messageId: string
  mentionedAgentType?: AgentType | null
  mentionedAgentId?: string | null
  operatorLabel?: string | null
  handled: boolean
  handledAt?: Iso8601 | null
  createdAt: Iso8601
}

export interface RoomMember {
  memberType: 'agent' | 'operator'
  online: boolean
  agentId?: string
  agentType?: AgentType
  operatorLabel?: string
}

export interface Room {
  roomId: string
  projectId: string
  status: 'active' | 'archived'
  operatorLabel: string
  members: RoomMember[]
}

export interface SendMessageInput {
  messageType: MessageType
  content: string
  payload?: Record<string, unknown>
}

// ───────────────────────────── WebSocket ─────────────────────────────

export type WsEventType =
  | 'room.connected'
  | 'room.member_online'
  | 'room.member_offline'
  | 'room.message'
  | 'task.updated'
  | 'context.handoff_created'
  | 'system.notice'

export interface WsEnvelope<P = unknown> {
  eventId: string
  eventType: WsEventType
  projectId: string
  roomId: string
  sender?: MessageSender
  payload: P
  createdAt: Iso8601
}

export type WsConnectionStatus = 'connecting' | 'open' | 'closed' | 'reconnecting'

// ───────────────────────────── Memory ─────────────────────────────

export type MemoryItemKind = 'markdown' | 'text' | 'directory' | 'json' | 'binary'

export interface MemoryItem {
  title: string
  uri: string
  type: MemoryItemKind
  size?: number
  updatedAt?: Iso8601
}

export interface MemoryListing {
  root: string
  items: MemoryItem[]
}

export interface MemorySearchHit {
  uri: string
  title: string
  snippet: string
  estimatedTokens?: number
}

export interface MemoryReadResponse {
  uri: string
  title: string
  contentType: string
  content: string
  updatedAt?: Iso8601
}

export type MemoryWriteTarget = 'decisions' | 'summary' | 'note' | 'handoff'

export interface MemoryWriteInput {
  target: MemoryWriteTarget
  title: string
  content: string
  relatedTaskId?: string
}

export interface MemoryWriteResponse {
  uri: string
  synced: boolean
}

// ───────────────────────────── Artifacts ─────────────────────────────

export type ArtifactType =
  | 'diff'
  | 'code_diff'
  | 'markdown'
  | 'pdf'
  | 'image'
  | 'json'
  | 'text'
  | 'other'

export interface ArtifactSummary {
  artifactId: string
  taskId?: string
  artifactType: ArtifactType
  name: string
  path: string
  size?: number
  createdAt: Iso8601
}

export interface ArtifactDetail extends ArtifactSummary {
  content: string
  metadata?: Record<string, unknown>
}
