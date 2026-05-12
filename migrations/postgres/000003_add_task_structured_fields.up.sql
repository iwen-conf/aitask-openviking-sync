-- Standardize delegated tasks around 6 structured fields:
--   title / goal / description (context) / inputs (input_context) /
--   constraints / output_contract (acceptance).
-- goal and constraints_text are new columns; description loses its NOT NULL so
-- "background" can be omitted when a task is already self-explanatory via goal.

ALTER TABLE tasks
  ADD COLUMN IF NOT EXISTS goal TEXT;

ALTER TABLE tasks
  ADD COLUMN IF NOT EXISTS constraints_text TEXT;

ALTER TABLE tasks
  ALTER COLUMN description DROP NOT NULL;
