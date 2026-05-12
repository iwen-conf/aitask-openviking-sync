-- Rollback initial schema. Drop in reverse dependency order.

DROP TABLE IF EXISTS system_openviking_settings;
DROP TABLE IF EXISTS project_progress_snapshots;
DROP TABLE IF EXISTS context_handoffs;
DROP TABLE IF EXISTS project_room_mentions;
DROP TABLE IF EXISTS project_room_messages;
DROP TABLE IF EXISTS project_room_members;
DROP TABLE IF EXISTS project_rooms;
DROP TABLE IF EXISTS artifacts;
DROP TABLE IF EXISTS task_events;
DROP TABLE IF EXISTS task_delegations;
DROP TABLE IF EXISTS task_dependencies;
DROP TABLE IF EXISTS task_required_skills;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS agent_run_context_usage;
DROP TABLE IF EXISTS agent_runs;
DROP TABLE IF EXISTS agent_models;
DROP TABLE IF EXISTS agent_skills;
DROP TABLE IF EXISTS agent_tokens;
DROP TABLE IF EXISTS agent_project_bindings;
ALTER TABLE IF EXISTS projects DROP CONSTRAINT IF EXISTS fk_projects_active_session;
DROP TABLE IF EXISTS project_sessions;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS agents;
