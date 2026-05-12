-- Backfill for deployments created before openviking_workspace_id existed.
ALTER TABLE projects
  ADD COLUMN IF NOT EXISTS openviking_workspace_id TEXT NOT NULL DEFAULT '';
