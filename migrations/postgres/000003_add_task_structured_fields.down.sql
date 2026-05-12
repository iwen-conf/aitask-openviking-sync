-- Reverse the structured-task migration.
-- Existing rows with NULL description are coerced to '' before re-imposing
-- NOT NULL so the constraint can be re-enabled cleanly.

UPDATE tasks
   SET description = ''
 WHERE description IS NULL;

ALTER TABLE tasks
  ALTER COLUMN description SET NOT NULL;

ALTER TABLE tasks
  DROP COLUMN IF EXISTS constraints_text;

ALTER TABLE tasks
  DROP COLUMN IF EXISTS goal;
