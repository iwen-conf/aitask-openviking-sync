-- AITask initial schema. Single migration: tables, FK actions, indexes.

CREATE TABLE agents (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  agent_type TEXT NOT NULL,
  default_model TEXT,
  status TEXT NOT NULL DEFAULT 'active',
  role TEXT NOT NULL DEFAULT 'worker',

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  goal TEXT,
  status TEXT NOT NULL DEFAULT 'active',

  created_by_label TEXT,
  active_session_id TEXT,

  openviking_namespace TEXT NOT NULL,
  openviking_root_uri TEXT NOT NULL,
  openviking_workspace_id TEXT NOT NULL DEFAULT '',

  completion_policy JSONB,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE project_sessions (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,

  status TEXT NOT NULL DEFAULT 'active',
  context_summary TEXT,
  current_phase TEXT,
  version INT NOT NULL DEFAULT 1,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE projects
  ADD CONSTRAINT fk_projects_active_session
  FOREIGN KEY (active_session_id)
  REFERENCES project_sessions(id)
  ON DELETE SET NULL
  ON UPDATE CASCADE
  DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE agent_project_bindings (
  agent_id TEXT NOT NULL
    REFERENCES agents(id) ON DELETE CASCADE ON UPDATE CASCADE,
  project_id TEXT NOT NULL
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,
  role TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,

  PRIMARY KEY (agent_id, project_id)
);

CREATE TABLE agent_tokens (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL
    REFERENCES agents(id) ON DELETE CASCADE ON UPDATE CASCADE,

  token_hash TEXT NOT NULL,
  scopes TEXT[] NOT NULL,

  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE agent_skills (
  agent_id TEXT NOT NULL
    REFERENCES agents(id) ON DELETE CASCADE ON UPDATE CASCADE,
  skill_name TEXT NOT NULL,

  PRIMARY KEY (agent_id, skill_name)
);

CREATE TABLE agent_models (
  agent_id TEXT NOT NULL
    REFERENCES agents(id) ON DELETE CASCADE ON UPDATE CASCADE,
  model_name TEXT NOT NULL,

  PRIMARY KEY (agent_id, model_name)
);

CREATE TABLE agent_runs (
  id TEXT PRIMARY KEY,

  agent_id TEXT NOT NULL
    REFERENCES agents(id) ON DELETE CASCADE ON UPDATE CASCADE,
  project_id TEXT NOT NULL
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,
  session_id TEXT NOT NULL
    REFERENCES project_sessions(id) ON DELETE CASCADE ON UPDATE CASCADE,

  status TEXT NOT NULL DEFAULT 'active',

  model_name TEXT,
  max_context_tokens INT,
  estimated_used_tokens INT NOT NULL DEFAULT 0,
  context_state TEXT NOT NULL DEFAULT 'normal',

  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ended_at TIMESTAMPTZ,
  end_reason TEXT,
  last_heartbeat_at TIMESTAMPTZ,

  metadata JSONB
);

CREATE TABLE agent_run_context_usage (
  id TEXT PRIMARY KEY,

  run_id TEXT NOT NULL
    REFERENCES agent_runs(id) ON DELETE CASCADE ON UPDATE CASCADE,
  project_id TEXT NOT NULL
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,

  source TEXT NOT NULL,

  estimated_input_tokens INT NOT NULL DEFAULT 0,
  estimated_output_tokens INT NOT NULL DEFAULT 0,
  reported_input_tokens INT,
  reported_output_tokens INT,

  total_estimated_tokens INT NOT NULL,
  max_context_tokens INT,
  state TEXT NOT NULL,

  payload JSONB,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE tasks (
  id TEXT PRIMARY KEY,

  project_id TEXT NOT NULL
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,
  session_id TEXT NOT NULL
    REFERENCES project_sessions(id) ON DELETE CASCADE ON UPDATE CASCADE,

  parent_task_id TEXT,

  title TEXT NOT NULL,
  description TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'planned',

  delegation_kind TEXT NOT NULL DEFAULT 'direct_agent',

  assignee_agent_id TEXT
    REFERENCES agents(id) ON DELETE SET NULL ON UPDATE CASCADE,
  assignee_agent_type TEXT,

  required_model TEXT,
  priority INT NOT NULL DEFAULT 0,

  delegated_by_type TEXT,
  delegated_by_agent_id TEXT
    REFERENCES agents(id) ON DELETE SET NULL ON UPDATE CASCADE,
  delegated_by_operator_label TEXT,
  delegated_at TIMESTAMPTZ,

  active_run_id TEXT
    REFERENCES agent_runs(id) ON DELETE SET NULL ON UPDATE CASCADE,
  started_at TIMESTAMPTZ,
  last_heartbeat_at TIMESTAMPTZ,

  input_context TEXT,
  output_contract TEXT,

  result TEXT,
  error TEXT,

  created_by_type TEXT NOT NULL,
  created_by_operator_label TEXT,
  created_by_agent_id TEXT,

  attempt_count INT NOT NULL DEFAULT 0,
  max_attempts INT NOT NULL DEFAULT 3,
  is_required BOOLEAN NOT NULL DEFAULT TRUE,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE task_required_skills (
  task_id TEXT NOT NULL
    REFERENCES tasks(id) ON DELETE CASCADE ON UPDATE CASCADE,
  skill_name TEXT NOT NULL,

  PRIMARY KEY (task_id, skill_name)
);

CREATE TABLE task_dependencies (
  task_id TEXT NOT NULL
    REFERENCES tasks(id) ON DELETE CASCADE ON UPDATE CASCADE,
  depends_on_task_id TEXT NOT NULL
    REFERENCES tasks(id) ON DELETE CASCADE ON UPDATE CASCADE,

  PRIMARY KEY (task_id, depends_on_task_id)
);

CREATE TABLE task_delegations (
  id TEXT PRIMARY KEY,

  task_id TEXT NOT NULL
    REFERENCES tasks(id) ON DELETE CASCADE ON UPDATE CASCADE,
  assignee_agent_id TEXT NOT NULL
    REFERENCES agents(id) ON DELETE CASCADE ON UPDATE CASCADE,
  delegated_by_type TEXT NOT NULL,
  delegated_by_agent_id TEXT
    REFERENCES agents(id) ON DELETE SET NULL ON UPDATE CASCADE,
  delegated_by_operator_label TEXT,
  reason TEXT,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE task_events (
  id TEXT PRIMARY KEY,

  project_id TEXT NOT NULL
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,
  session_id TEXT NOT NULL
    REFERENCES project_sessions(id) ON DELETE CASCADE ON UPDATE CASCADE,
  task_id TEXT
    REFERENCES tasks(id) ON DELETE SET NULL ON UPDATE CASCADE,

  event_type TEXT NOT NULL,

  from_status TEXT,
  to_status TEXT,

  actor_type TEXT NOT NULL,
  actor_operator_label TEXT,
  actor_agent_id TEXT,
  actor_run_id TEXT,

  payload JSONB,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE artifacts (
  id TEXT PRIMARY KEY,

  project_id TEXT NOT NULL
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,
  session_id TEXT NOT NULL
    REFERENCES project_sessions(id) ON DELETE CASCADE ON UPDATE CASCADE,
  task_id TEXT
    REFERENCES tasks(id) ON DELETE SET NULL ON UPDATE CASCADE,

  artifact_type TEXT NOT NULL CHECK (
    artifact_type IN ('code_diff', 'doc', 'report', 'image')
  ),
  name TEXT NOT NULL,
  path TEXT,
  content TEXT,
  metadata JSONB,

  created_by_agent_id TEXT,
  created_by_run_id TEXT,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE project_rooms (
  id TEXT PRIMARY KEY,

  project_id TEXT NOT NULL UNIQUE
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,
  status TEXT NOT NULL DEFAULT 'active',
  room_type TEXT NOT NULL DEFAULT 'agent_room',

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE project_room_members (
  id TEXT PRIMARY KEY,

  room_id TEXT NOT NULL
    REFERENCES project_rooms(id) ON DELETE CASCADE ON UPDATE CASCADE,
  project_id TEXT NOT NULL
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,

  member_type TEXT NOT NULL,
  operator_label TEXT,
  agent_id TEXT,
  agent_type TEXT,
  role TEXT NOT NULL,

  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ,

  UNIQUE (room_id, operator_label),
  UNIQUE (room_id, agent_id)
);

CREATE TABLE project_room_messages (
  id TEXT PRIMARY KEY,

  room_id TEXT NOT NULL
    REFERENCES project_rooms(id) ON DELETE CASCADE ON UPDATE CASCADE,
  project_id TEXT NOT NULL
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,

  sender_type TEXT NOT NULL,
  sender_operator_label TEXT,
  sender_agent_id TEXT,
  sender_agent_type TEXT,

  message_type TEXT NOT NULL,
  content TEXT,
  payload JSONB,

  related_task_id TEXT,
  related_artifact_id TEXT,

  visibility TEXT NOT NULL DEFAULT 'room',

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE project_room_mentions (
  id TEXT PRIMARY KEY,

  room_id TEXT NOT NULL
    REFERENCES project_rooms(id) ON DELETE CASCADE ON UPDATE CASCADE,
  message_id TEXT NOT NULL
    REFERENCES project_room_messages(id) ON DELETE CASCADE ON UPDATE CASCADE,

  mentioned_agent_type TEXT,
  mentioned_agent_id TEXT,
  mentioned_operator_label TEXT,

  handled BOOLEAN NOT NULL DEFAULT FALSE,
  handled_at TIMESTAMPTZ,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE context_handoffs (
  id TEXT PRIMARY KEY,

  project_id TEXT NOT NULL
    REFERENCES projects(id) ON DELETE CASCADE ON UPDATE CASCADE,
  session_id TEXT NOT NULL
    REFERENCES project_sessions(id) ON DELETE CASCADE ON UPDATE CASCADE,

  from_agent_id TEXT NOT NULL
    REFERENCES agents(id) ON DELETE CASCADE ON UPDATE CASCADE,
  from_run_id TEXT NOT NULL
    REFERENCES agent_runs(id) ON DELETE CASCADE ON UPDATE CASCADE,

  to_agent_id TEXT,
  to_agent_type TEXT,

  task_id TEXT
    REFERENCES tasks(id) ON DELETE SET NULL ON UPDATE CASCADE,
  status TEXT NOT NULL DEFAULT 'created',
  reason TEXT NOT NULL,

  summary TEXT NOT NULL,
  next_steps JSONB,
  openviking_refs JSONB,
  artifact_refs JSONB,
  local_state JSONB,
  openviking_uri TEXT,

  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  consumed_by_run_id TEXT,
  consumed_at TIMESTAMPTZ
);

CREATE TABLE project_progress_snapshots (
  project_id TEXT PRIMARY KEY
    REFERENCES projects(id) ON DELETE CASCADE,
  done_count INT NOT NULL DEFAULT 0,
  total_count INT NOT NULL DEFAULT 0,
  blocked_count INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE system_openviking_settings (
  id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id = TRUE),
  server_url TEXT NOT NULL DEFAULT '',
  api_key_ciphertext BYTEA,
  enable_memory_write BOOLEAN NOT NULL DEFAULT TRUE,
  enable_auto_sync BOOLEAN NOT NULL DEFAULT TRUE,
  last_sync_at TIMESTAMPTZ,
  last_error TEXT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_by TEXT NOT NULL DEFAULT ''
);

-- Indexes.

CREATE INDEX idx_projects_status_updated_at
  ON projects (status, updated_at DESC);

CREATE INDEX idx_projects_active_session_id
  ON projects (active_session_id);

CREATE INDEX idx_project_sessions_project_status_updated_at
  ON project_sessions (project_id, status, updated_at DESC);

CREATE INDEX idx_agent_project_bindings_project_enabled_agent
  ON agent_project_bindings (project_id, enabled, agent_id);

CREATE INDEX idx_agent_project_bindings_project_agent_enabled
  ON agent_project_bindings (project_id, agent_id, enabled);

CREATE INDEX idx_agent_tokens_agent_revoked_expires
  ON agent_tokens (agent_id, revoked_at, expires_at DESC);

CREATE INDEX idx_agent_tokens_token_hash
  ON agent_tokens (token_hash);

CREATE INDEX idx_agent_runs_project_status_started_at
  ON agent_runs (project_id, status, started_at DESC);

CREATE INDEX idx_agent_runs_agent_project_status
  ON agent_runs (agent_id, project_id, status);

CREATE INDEX idx_agent_runs_session_started_at
  ON agent_runs (session_id, started_at DESC);

CREATE INDEX idx_agent_run_context_usage_run_id_created_at
  ON agent_run_context_usage (run_id, created_at);

CREATE INDEX idx_agent_run_context_usage_project_created_at
  ON agent_run_context_usage (project_id, created_at DESC);

CREATE INDEX idx_tasks_project_status_assignee
  ON tasks (project_id, status, assignee_agent_id);

CREATE INDEX idx_tasks_assignee_active_run
  ON tasks (assignee_agent_id, active_run_id);

CREATE INDEX idx_tasks_project_updated_at
  ON tasks (project_id, updated_at DESC);

CREATE INDEX idx_tasks_assignee_status_updated_at
  ON tasks (assignee_agent_id, status, updated_at DESC);

CREATE INDEX idx_tasks_active_run_id
  ON tasks (active_run_id);

CREATE INDEX idx_tasks_session_status_updated_at
  ON tasks (session_id, status, updated_at DESC);

CREATE INDEX idx_task_required_skills_skill_task
  ON task_required_skills (skill_name, task_id);

CREATE INDEX idx_task_required_skills_task_skill
  ON task_required_skills (task_id, skill_name);

CREATE INDEX idx_task_dependencies_depends_on_task
  ON task_dependencies (depends_on_task_id, task_id);

CREATE INDEX idx_task_dependencies_task_dep
  ON task_dependencies (task_id, depends_on_task_id);

CREATE INDEX idx_task_delegations_task_created_at
  ON task_delegations (task_id, created_at DESC);

CREATE INDEX idx_task_delegations_assignee_created_at
  ON task_delegations (assignee_agent_id, created_at DESC);

CREATE INDEX idx_task_events_project_created_at
  ON task_events (project_id, created_at);

CREATE INDEX idx_task_events_task_created_at
  ON task_events (task_id, created_at DESC);

CREATE INDEX idx_task_events_session_created_at
  ON task_events (session_id, created_at DESC);

CREATE INDEX idx_artifacts_project_created_at
  ON artifacts (project_id, created_at DESC);

CREATE INDEX idx_artifacts_session_created_at
  ON artifacts (session_id, created_at DESC);

CREATE INDEX idx_artifacts_task_created_at
  ON artifacts (task_id, created_at DESC);

CREATE INDEX idx_artifacts_project_task_type
  ON artifacts (project_id, task_id, artifact_type);

CREATE INDEX idx_project_room_members_project_member_joined
  ON project_room_members (project_id, member_type, joined_at DESC);

CREATE INDEX idx_project_room_messages_room_created_at
  ON project_room_messages (room_id, created_at DESC);

CREATE INDEX idx_project_room_messages_project_created_at
  ON project_room_messages (project_id, created_at DESC);

CREATE INDEX idx_project_room_mentions_room_handled_created_at
  ON project_room_mentions (room_id, handled, created_at DESC);

CREATE INDEX idx_project_room_mentions_message_id
  ON project_room_mentions (message_id);

CREATE INDEX idx_context_handoffs_project_status_created_at
  ON context_handoffs (project_id, status, created_at DESC);

CREATE INDEX idx_context_handoffs_session_created_at
  ON context_handoffs (session_id, created_at DESC);

CREATE INDEX idx_context_handoffs_task_created_at
  ON context_handoffs (task_id, created_at DESC);

CREATE INDEX idx_context_handoffs_from_run_id
  ON context_handoffs (from_run_id);

CREATE INDEX idx_project_progress_snapshots_updated_at
  ON project_progress_snapshots (updated_at DESC);
