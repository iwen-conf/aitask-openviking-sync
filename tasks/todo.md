# Delegation Model Migration Plan

## Scope

- Replace task claiming / pulling as the authoritative work acquisition model.
- Define task delegation as the authoritative model: a coordinator/operator delegates a task to a concrete agent or agent type, and the target agent only reads/accepts delegated work.
- Remove lease-centric ownership language from contracts and task plans; use assignment/delegation ownership instead.
- Keep unrelated meanings of "pull" such as skill sync intact.
- Verify docs and backend scaffold after edits.

## Checklist

- [x] Locate claim/pull/lease references in docs and backend.
- [x] Preserve existing frontend draft because it already uses `delegatedBy` / `assigneeId` and is user-owned untracked work.
- [x] Update canonical requirements document.
- [x] Update API contract docs.
- [x] Update backend/frontend/review task lists and status flow wording.
- [x] Update backend code with delegation model guardrails.
- [x] Run `go test ./...` in `backend/`.
- [x] Run terminology scan for stale authoritative claim/lease language.

## Review Notes

- `go test ./...` passed in `backend/`.
- Terminology scan has no remaining authoritative claim/lease task model references; remaining matches are the lessons anti-pattern notes or unrelated words such as `Release`.

---

# RV Runtime / Perf / Diff Harness

## Scope

- Add reusable review harness scripts that can boot the real Docker Compose stack and collect review evidence under `.artifacts/review/`.
- Cover runtime validation for `RV-001~009` and `RV-020~029`, performance evidence for `RV-040~045`, and automated diff checks for `RV-051`, `RV-054`, `RV-056~059`.
- Keep the existing static `scripts/review-gate.sh` checks, but extend it to orchestrate the new runtime/perf/diff phases.

## Checklist

- [ ] Add shared shell helpers for env/bootstrap/artifact handling.
- [ ] Add real runtime review script against Docker Compose + PostgreSQL + DragonflyDB + OpenViking.
- [ ] Add performance evidence script with seeded scale data and captured reports.
- [ ] Add automated diff script for schema / scope / OpenViking / `.aitask` / WS event consistency.
- [ ] Wire the new scripts into `scripts/review-gate.sh`.
- [ ] Run targeted verification and capture residual environment blockers if any.

---

# BE-001 Docker Compose Baseline

## Scope

- Add root `docker-compose.yml` with mandatory services: `postgres`, `dragonfly`, `backend`, `web`, `openviking`.
- Keep BE-003/FE-006 independent by using runtime images for `backend` and `web` in this phase.
- Validate compose syntax and run-state health.

## Checklist

- [x] Add `docker-compose.yml` baseline with PostgreSQL `18.3` and DragonflyDB `v1.38.1`.
- [x] Add placeholder `openviking` service slot.
- [x] Add container healthchecks for all services.
- [x] Validate with `docker compose config`.
- [x] Boot stack and verify all services become `running` + `healthy` (using temporary host port overrides due local port conflict).
- [x] Move `BE-001` from `未完成.md` to `待审查.md`.

## Review Notes

- `docker compose -f docker-compose.yml config` passed.
- Default host `6379` was already occupied on this machine, so runtime verification used:
  `APP_PORT=18080 WEB_PORT=13000 POSTGRES_PORT=15432 DRAGONFLY_PORT=16379 docker compose up -d`.
- After startup, `docker compose ps --format json` reported all five services healthy.

---

# BE-002 .env Example + Config Loading

## Scope

- Add root `.env.example` aligned with spec §11.3.
- Extend backend config loader to read infrastructure/auth/operator config keys.
- Prove that copying `.env.example` to `.env` enables compose/backend config loading.

## Checklist

- [x] Add root `.env.example` with `POSTGRES_*`, `DRAGONFLY_URL`, `AGENT_TOKEN_SECRET`, `OPENVIKING_BASE_URL`, `CONSOLE_OPERATOR_LABEL`.
- [x] Extend `backend/internal/config` to load and validate DB/cache/openviking/security/operator keys.
- [x] Update tests for defaults, overrides, and invalid env values.
- [x] Wire backend compose service to consume `.env` and pass relevant keys.
- [x] Run `go test ./...` in `backend/`.
- [x] Verify `cp .env.example .env && docker compose -f docker-compose.yml config` succeeds.
- [x] Verify backend process boots with `.env.example` values and `/healthz` returns 200.

## Review Notes

- `go test ./...` passed in `backend/` after config changes.
- `docker compose -f docker-compose.yml config` now requires `.env` (as intended for BE-002 flow via `env_file`).
- Runtime verification command returned 200:
  `set -a && source .env.example && AITASK_HTTP_PORT=18081 go run ./backend/cmd/server` then `GET /healthz`.

---

# BE-005 Dev Bootstrap Scripts

## Scope

- Add `scripts/dev-up.sh` to orchestrate environment bootstrap (`docker compose up`, migration, readiness check).
- Add `scripts/migrate.sh` to apply SQL files under `migrations/postgres` against PostgreSQL 18.3 in the running compose stack.
- Keep behavior safe for current baseline where migration files may be absent, while staying compatible with upcoming BE-010 migration tooling.

## Checklist

- [x] Add `scripts/migrate.sh` with deterministic SQL execution order and clear no-op behavior when no migration SQL exists.
- [x] Add `scripts/dev-up.sh` with `.env` bootstrap, `docker compose up -d --build`, migration invocation, and `/readyz` polling.
- [x] Validate from repo root: `bash scripts/dev-up.sh` reaches healthy backend state.
- [x] Move `BE-005` from `进行中.md` to `待审查.md`.

## Review Notes

- Added executable scripts: `scripts/dev-up.sh`, `scripts/migrate.sh`.
- `migrate.sh` waits for postgres health and applies `migrations/postgres/*.sql` in sorted order; when no SQL exists it prints skip and exits 0.
- `dev-up.sh` supports:
  `ENV_FILE` (custom env file), `DEV_UP_BUILD=0` (skip image rebuild), `DEV_UP_SERVICES` (service subset), `READYZ_WAIT_SECONDS`.
- Runtime verification succeeded with local port override env file:
  `ENV_FILE=.env.devup.local DEV_UP_BUILD=0 bash scripts/dev-up.sh`.
- On this machine, default `bash scripts/dev-up.sh` failed due environmental constraints (host port 5432/6379 already allocated, and transient upstream registry EOF while pulling builder/runtime base images).

---

# BE-006 API Baseline + Mock Assets

## Scope

- Keep `docs/API/*.md` as source-of-truth contract.
- Add `api/` deliverables required by BE-006:
  OpenAPI draft, Protobuf service draft, WebSocket envelope JSON Schema, and module mock responses.
- Cover Project / Task / Agent / Room / Memory / Context / Health, without any user-management endpoints.

## Checklist

- [x] Create `api/openapi/openapi.yaml` mapped from `docs/API/*.md`.
- [x] Create Protobuf draft under `api/protobuf/aitask/v1/`.
- [x] Create WebSocket envelope schema under `api/websocket/`.
- [x] Create JSON mock fixtures under `api/mock/`.
- [x] Verify artifacts are internally consistent and align with `docs/API/README.md` constraints.
- [x] Move `BE-006` from `进行中.md` to `待审查.md`.

## Review Notes

- Added machine-readable API drafts under `api/`:
  - OpenAPI: `api/openapi/openapi.yaml`
  - Protobuf draft: `api/protobuf/aitask/v1/*.proto`
  - WebSocket schema: `api/websocket/agent-room-envelope.schema.json`
  - Mocks: `api/mock/*.json`
- Updated `docs/API/README.md` with the generated artifact locations.
- Validation results:
  - `ruby -e 'require \"yaml\"; YAML.load_file(\"api/openapi/openapi.yaml\")'` passed
  - JSON parse validation for `api/mock/*.json` and `api/websocket/*.json` passed
- `protoc` not installed in this environment, so proto compile validation is deferred to BE-090 — BE-099 toolchain stage.

---

# BE-010 Migration Tooling Baseline

## Scope

- Select one migration tool (golang-migrate/goose/atlas) and document decision.
- Keep migration files under `migrations/postgres/`.
- Provide command entrypoint that supports versioned `up/down/goto/version`.
- Ensure BE-005 `dev-up.sh` still works with default migration command.

## Checklist

- [x] Document migration tool decision and operational constraints.
- [x] Add versioned migration baseline files under `migrations/postgres/`.
- [x] Upgrade `scripts/migrate.sh` to support `up/down/goto/version` at arbitrary versions.
- [x] Verify `up` and `down` by explicit version control against local PostgreSQL 18.3 container.
- [x] Move `BE-010` from `进行中.md` to `待审查.md`.

## Review Notes

- Selected `golang-migrate`; decision documented in `docs/后端/decisions.md`.
- Added migration baseline files:
  `migrations/postgres/000001_be010_migration_tooling_probe.up.sql` and
  `migrations/postgres/000001_be010_migration_tooling_probe.down.sql`.
- Upgraded `scripts/migrate.sh` command surface:
  - `up [N]`
  - `down [N]`
  - `goto VERSION`
  - `force VERSION`
  - `version`
- Compatibility: `scripts/dev-up.sh` unchanged in calling convention (`bash scripts/migrate.sh` defaults to `up`).
- Runtime verification (with local env override `.env.be010.local`):
  - `docker compose ... up -d postgres`
  - `ENV_FILE=.env.be010.local bash scripts/migrate.sh up` -> applied version 1
  - `ENV_FILE=.env.be010.local bash scripts/migrate.sh down 1` -> reverted version 1
  - `ENV_FILE=.env.be010.local bash scripts/migrate.sh goto 1` -> re-applied version 1
  - `ENV_FILE=.env.be010.local bash scripts/migrate.sh version` -> `1`
  - cleanup via `docker compose ... down`

---

# BE-011 Projects + Project Sessions Tables

## Scope

- Add versioned migration pair for `projects` and `project_sessions` from spec §12.1/§12.2.
- Ensure bidirectional reference is supported by adding FK from `projects.active_session_id` to `project_sessions.id`.
- Verify migration lifecycle (`up/down/goto/version`) against local PostgreSQL 18.3.

## Checklist

- [x] Add `000002_be011_projects_sessions.up.sql` with both table definitions.
- [x] Add `000002_be011_projects_sessions.down.sql` to cleanly rollback BE-011 objects.
- [x] Add FK `projects.active_session_id -> project_sessions.id`.
- [x] Verify `up` to version `2` and confirm schema objects via `psql`.
- [x] Verify `down 1` and `goto 2` replay works.
- [x] Move `BE-011` from `进行中.md` to `待审查.md`.

## Review Notes

- Added migration files:
  - `migrations/postgres/000002_be011_projects_sessions.up.sql`
  - `migrations/postgres/000002_be011_projects_sessions.down.sql`
- `up` migration creates:
  - `projects` table with fields from spec §12.1.
  - `project_sessions` table with fields from spec §12.2 and FK `project_sessions.project_id -> projects.id`.
  - FK constraint `fk_projects_active_session` from `projects.active_session_id` to `project_sessions.id` (`DEFERRABLE INITIALLY DEFERRED`).
- Runtime verification used isolated env file `/tmp/aitask-be011.env` to avoid host port conflicts:
  - `docker compose -f docker-compose.yml --env-file /tmp/aitask-be011.env up -d postgres`
  - `ENV_FILE=/tmp/aitask-be011.env bash scripts/migrate.sh up` -> applied version `2`
  - `ENV_FILE=/tmp/aitask-be011.env bash scripts/migrate.sh version` -> `2`
  - `psql` check confirmed both tables and constraint:
    - `tables=project_sessions,projects`
    - `fk_projects_active_session: FOREIGN KEY (active_session_id) REFERENCES project_sessions(id) DEFERRABLE INITIALLY DEFERRED`
  - `ENV_FILE=/tmp/aitask-be011.env bash scripts/migrate.sh down 1` -> reverted version `2`
  - `ENV_FILE=/tmp/aitask-be011.env bash scripts/migrate.sh goto 2` -> reapplied version `2`
  - final `ENV_FILE=/tmp/aitask-be011.env bash scripts/migrate.sh version` -> `2`
  - cleanup: `docker compose -f docker-compose.yml --env-file /tmp/aitask-be011.env down`

---

# BE-012 Agent Schema Tables

## Scope

- Add versioned migration pair for `agents`, `agent_project_bindings`, `agent_tokens`, `agent_skills`, `agent_models` from spec §12.3–§12.7.
- Preserve FK relationships to `projects` and `agents`.
- Verify migration lifecycle and acceptance requirement that `agent_tokens.token_hash` is `NOT NULL`.

## Checklist

- [x] Add `000003_be012_agent_system.up.sql` with five table definitions.
- [x] Add `000003_be012_agent_system.down.sql` with dependency-safe rollback order.
- [x] Verify migration `up` reaches version `3`.
- [x] Verify all five agent tables exist after migration.
- [x] Verify `agent_tokens.token_hash` is non-nullable and no plaintext token column is present.
- [x] Verify `down 1` and `goto 3` replay behavior.
- [x] Move `BE-012` from `进行中.md` to `待审查.md`.

## Review Notes

- Added migration files:
  - `migrations/postgres/000003_be012_agent_system.up.sql`
  - `migrations/postgres/000003_be012_agent_system.down.sql`
- `up` migration creates:
  - `agents`
  - `agent_project_bindings`
  - `agent_tokens`
  - `agent_skills`
  - `agent_models`
- Runtime verification used isolated env file `/tmp/aitask-be012.env`:
  - `docker compose -f docker-compose.yml --env-file /tmp/aitask-be012.env up -d postgres`
  - `ENV_FILE=/tmp/aitask-be012.env bash scripts/migrate.sh up` -> applied version `3`
  - `ENV_FILE=/tmp/aitask-be012.env bash scripts/migrate.sh version` -> `3`
  - `psql` checks:
    - `tables=agent_models,agent_project_bindings,agent_skills,agent_tokens,agents`
    - `token_hash:NO` (`NO` = non-nullable)
    - `token_columns=0` (no plaintext token-like column)
  - `ENV_FILE=/tmp/aitask-be012.env bash scripts/migrate.sh down 1` -> reverted version `3`
  - `ENV_FILE=/tmp/aitask-be012.env bash scripts/migrate.sh goto 3` -> reapplied version `3`
  - final `ENV_FILE=/tmp/aitask-be012.env bash scripts/migrate.sh version` -> `3`
  - cleanup: `docker compose -f docker-compose.yml --env-file /tmp/aitask-be012.env down`

---

# BE-013 Agent Runs + Context Usage Tables

## Scope

- Add versioned migration pair for `agent_runs` and `agent_run_context_usage` from spec §12.8/§12.20.
- Add required index on `agent_run_context_usage(run_id, created_at)`.
- Verify acceptance that `agent_runs.context_state` defaults to `normal`.

## Checklist

- [x] Add `000004_be013_agent_runs_context_usage.up.sql` with both table definitions.
- [x] Add `000004_be013_agent_runs_context_usage.down.sql` with rollback-safe drop order.
- [x] Add index `(run_id, created_at)` for context usage lookup path.
- [x] Verify migration `up` reaches version `4`.
- [x] Verify both tables exist and index definition is present.
- [x] Verify `agent_runs.context_state` default is `normal`.
- [x] Verify `down 1` and `goto 4` replay behavior.
- [x] Move `BE-013` from `进行中.md` to `待审查.md`.

## Review Notes

- Added migration files:
  - `migrations/postgres/000004_be013_agent_runs_context_usage.up.sql`
  - `migrations/postgres/000004_be013_agent_runs_context_usage.down.sql`
- `up` migration creates:
  - `agent_runs`
  - `agent_run_context_usage`
  - `idx_agent_run_context_usage_run_id_created_at` on `(run_id, created_at)`
- Runtime verification used isolated env file `/tmp/aitask-be013.env`:
  - `docker compose -f docker-compose.yml --env-file /tmp/aitask-be013.env up -d postgres`
  - `ENV_FILE=/tmp/aitask-be013.env bash scripts/migrate.sh up` -> applied version `4`
  - `ENV_FILE=/tmp/aitask-be013.env bash scripts/migrate.sh version` -> `4`
  - `psql` checks:
    - `tables=agent_run_context_usage,agent_runs`
    - `context_state:'normal'::text`
    - `idx_agent_run_context_usage_run_id_created_at:CREATE INDEX ... (run_id, created_at)`
  - `ENV_FILE=/tmp/aitask-be013.env bash scripts/migrate.sh down 1` -> reverted version `4`
  - `ENV_FILE=/tmp/aitask-be013.env bash scripts/migrate.sh goto 4` -> reapplied version `4`
  - final `ENV_FILE=/tmp/aitask-be013.env bash scripts/migrate.sh version` -> `4`
  - cleanup: `docker compose -f docker-compose.yml --env-file /tmp/aitask-be013.env down`

---

# BE-014 Tasks + Required Skills + Dependencies Tables

## Scope

- Add versioned migration pair for `tasks`, `task_required_skills`, and `task_dependencies` from spec §12.9/§12.10/§12.11.
- Add required indexes for delegated-task lookup:
  - `(project_id, status, assignee_agent_id)`
  - `(assignee_agent_id, active_run_id)`
- Validate migration lifecycle and basic performance target evidence with 100k-row sample query.

## Checklist

- [x] Add `000005_be014_tasks_and_relations.up.sql` with all three table definitions.
- [x] Add `000005_be014_tasks_and_relations.down.sql` with rollback-safe drop order.
- [x] Add index `idx_tasks_project_status_assignee`.
- [x] Add index `idx_tasks_assignee_active_run`.
- [x] Verify migration `up` reaches version `5`.
- [x] Verify tables/defaults/indexes exist in PostgreSQL.
- [x] Run 100k-row transactional sample query and capture execution time.
- [x] Verify `down 1` and `goto 5` replay behavior.
- [x] Move `BE-014` from `进行中.md` to `待审查.md`.

## Review Notes

- Added migration files:
  - `migrations/postgres/000005_be014_tasks_and_relations.up.sql`
  - `migrations/postgres/000005_be014_tasks_and_relations.down.sql`
- `up` migration creates:
  - `tasks`
  - `task_required_skills`
  - `task_dependencies`
  - `idx_tasks_project_status_assignee` on `(project_id, status, assignee_agent_id)`
  - `idx_tasks_assignee_active_run` on `(assignee_agent_id, active_run_id)`
- Runtime verification used isolated env file `/tmp/aitask-be014.env`:
  - `docker compose -f docker-compose.yml --env-file /tmp/aitask-be014.env up -d postgres`
  - `ENV_FILE=/tmp/aitask-be014.env bash scripts/migrate.sh up` -> applied version `5`
  - `ENV_FILE=/tmp/aitask-be014.env bash scripts/migrate.sh version` -> `5`
  - `psql` checks:
    - `tables=task_dependencies,task_required_skills,tasks`
    - `status:'planned'::text`
    - both task lookup indexes present in `pg_indexes`
  - 100k-row transactional sample (`BEGIN ... INSERT generate_series ... EXPLAIN ANALYZE ... ROLLBACK`) yielded:
    - `Execution Time: 0.016 ms`
  - `ENV_FILE=/tmp/aitask-be014.env bash scripts/migrate.sh down 1` -> reverted version `5`
  - `ENV_FILE=/tmp/aitask-be014.env bash scripts/migrate.sh goto 5` -> reapplied version `5`
  - final `ENV_FILE=/tmp/aitask-be014.env bash scripts/migrate.sh version` -> `5`
  - cleanup: `docker compose -f docker-compose.yml --env-file /tmp/aitask-be014.env down`

---

# BE-015 Task Delegations + Task Events Tables

## Scope

- Add versioned migration pair for `task_delegations` and `task_events` from spec §12.12/§12.13.
- Ensure acceptance constraints:
  - `task_delegations.assignee_agent_id` must be `NOT NULL`.
  - `task_events` must have index on `(project_id, created_at)`.
- Verify migration lifecycle through `up/down/goto/version`.

## Checklist

- [x] Add `000006_be015_task_delegations_events.up.sql` with both table definitions.
- [x] Add `000006_be015_task_delegations_events.down.sql` with rollback-safe order.
- [x] Add `idx_task_events_project_created_at`.
- [x] Verify migration `up` reaches version `6`.
- [x] Verify both tables and index exist in PostgreSQL.
- [x] Verify `task_delegations.assignee_agent_id` is non-nullable.
- [x] Verify `down 1` and `goto 6` replay behavior.
- [x] Move `BE-015` from `进行中.md` to `待审查.md`.

## Review Notes

- Added migration files:
  - `migrations/postgres/000006_be015_task_delegations_events.up.sql`
  - `migrations/postgres/000006_be015_task_delegations_events.down.sql`
- `up` migration creates:
  - `task_delegations`
  - `task_events`
  - `idx_task_events_project_created_at` on `(project_id, created_at)`
- Runtime verification used isolated env file `/tmp/aitask-be015.env`:
  - `docker compose -f docker-compose.yml --env-file /tmp/aitask-be015.env up -d postgres`
  - `ENV_FILE=/tmp/aitask-be015.env bash scripts/migrate.sh up` -> applied version `6`
  - `ENV_FILE=/tmp/aitask-be015.env bash scripts/migrate.sh version` -> `6`
  - `psql` checks:
    - `tables=task_delegations,task_events`
    - `assignee_agent_id:NO` (`NO` = non-nullable)
    - `idx_task_events_project_created_at:CREATE INDEX ... (project_id, created_at)`
  - `ENV_FILE=/tmp/aitask-be015.env bash scripts/migrate.sh down 1` -> reverted version `6`
  - `ENV_FILE=/tmp/aitask-be015.env bash scripts/migrate.sh goto 6` -> reapplied version `6`
  - final `ENV_FILE=/tmp/aitask-be015.env bash scripts/migrate.sh version` -> `6`
  - cleanup: `docker compose -f docker-compose.yml --env-file /tmp/aitask-be015.env down`

---

# BE-016 Artifacts Table

## Scope

- Add versioned migration pair for `artifacts` from spec §12.14.
- Enforce `artifact_type` enum baseline including at least:
  - `code_diff`
  - `doc`
  - `report`
  - `image`
- Verify artifact association path supports both:
  - linked to a concrete task (`task_id` present)
  - linked at session level (`task_id` absent)

## Checklist

- [x] Add `000007_be016_artifacts.up.sql` with full table definition.
- [x] Add `000007_be016_artifacts.down.sql` rollback file.
- [x] Add `artifact_type` CHECK enum constraint covering required values.
- [x] Verify migration `up` reaches version `7`.
- [x] Verify table and enum constraint exist in PostgreSQL.
- [x] Verify association behavior with `task_id` present and absent.
- [x] Verify `down 1` and `goto 7` replay behavior.
- [x] Move `BE-016` from `进行中.md` to `待审查.md`.

## Review Notes

- Added migration files:
  - `migrations/postgres/000007_be016_artifacts.up.sql`
  - `migrations/postgres/000007_be016_artifacts.down.sql`
- `up` migration creates:
  - `artifacts`
  - CHECK constraint for `artifact_type IN ('code_diff','doc','report','image')`
- Runtime verification used isolated env file `/tmp/aitask-be016.env`:
  - `docker compose -f docker-compose.yml --env-file /tmp/aitask-be016.env up -d postgres`
  - `ENV_FILE=/tmp/aitask-be016.env bash scripts/migrate.sh up` -> applied version `7`
  - `ENV_FILE=/tmp/aitask-be016.env bash scripts/migrate.sh version` -> `7`
  - `psql` checks:
    - `tables=artifacts`
    - `artifacts_artifact_type_check:CHECK ((artifact_type = ANY (...)))`
  - association check in one transaction:
    - inserted one artifact with `task_id` and one artifact with `task_id NULL`
    - result `with_task=1,without_task=1`
    - `ROLLBACK` (no residual data)
  - `ENV_FILE=/tmp/aitask-be016.env bash scripts/migrate.sh down 1` -> reverted version `7`
  - `ENV_FILE=/tmp/aitask-be016.env bash scripts/migrate.sh goto 7` -> reapplied version `7`
  - final `ENV_FILE=/tmp/aitask-be016.env bash scripts/migrate.sh version` -> `7`
  - cleanup: `docker compose -f docker-compose.yml --env-file /tmp/aitask-be016.env down`

---

# BE-017 Project Room Tables

## Scope

- Add versioned migration pair for Room system tables from spec §12.15–§12.18:
  - `project_rooms`
  - `project_room_members`
  - `project_room_messages`
  - `project_room_mentions`
- Enforce one-room-per-project with `UNIQUE(project_id)` on `project_rooms`.
- Verify migration lifecycle and duplicate-room rejection behavior.

## Checklist

- [x] Add `000008_be017_project_room_system.up.sql` with four room table definitions.
- [x] Add `000008_be017_project_room_system.down.sql` with dependency-safe drop order.
- [x] Verify migration `up` reaches version `8`.
- [x] Verify all four room tables exist.
- [x] Verify unique constraint on `project_rooms.project_id` exists.
- [x] Verify inserting a second room for the same project is rejected.
- [x] Verify `down 1` and `goto 8` replay behavior.
- [x] Move `BE-017` from `进行中.md` to `待审查.md`.

## Review Notes

- Added migration files:
  - `migrations/postgres/000008_be017_project_room_system.up.sql`
  - `migrations/postgres/000008_be017_project_room_system.down.sql`
- `up` migration creates:
  - `project_rooms` (`project_id` with UNIQUE constraint)
  - `project_room_members`
  - `project_room_messages`
  - `project_room_mentions`
- Runtime verification used isolated env file `/tmp/aitask-be017.env`:
  - `docker compose -f docker-compose.yml --env-file /tmp/aitask-be017.env up -d postgres`
  - `ENV_FILE=/tmp/aitask-be017.env bash scripts/migrate.sh up` -> applied version `8`
  - `ENV_FILE=/tmp/aitask-be017.env bash scripts/migrate.sh version` -> `8`
  - `psql` checks:
    - `tables=project_room_members,project_room_mentions,project_room_messages,project_rooms`
    - `project_rooms_project_id_key:UNIQUE (project_id)`
  - duplicate-room transactional check:
    - inserted first room for one project
    - second insert hit unique violation path
    - `NOTICE: duplicate_blocked=t`
    - `ROLLBACK` (no residual data)
  - `ENV_FILE=/tmp/aitask-be017.env bash scripts/migrate.sh down 1` -> reverted version `8`
  - `ENV_FILE=/tmp/aitask-be017.env bash scripts/migrate.sh goto 8` -> reapplied version `8`
  - final `ENV_FILE=/tmp/aitask-be017.env bash scripts/migrate.sh version` -> `8`
  - cleanup: `docker compose -f docker-compose.yml --env-file /tmp/aitask-be017.env down`

---

# BE-018 Context Handoffs Table

## Scope

- Add versioned migration pair for `context_handoffs` from spec §12.19.
- Preserve default semantics:
  - `status` default `created`
  - `consumed_by_run_id` default NULL
  - `consumed_at` default NULL
- Verify migration lifecycle and default behavior with transactional insert.

## Checklist

- [x] Add `000009_be018_context_handoffs.up.sql` with full table definition.
- [x] Add `000009_be018_context_handoffs.down.sql` rollback file.
- [x] Verify migration `up` reaches version `9`.
- [x] Verify table exists and key defaults match requirements.
- [x] Verify inserted row defaults `status=created` and `consumed_*` NULL.
- [x] Verify `down 1` and `goto 9` replay behavior.
- [x] Move `BE-018` from `进行中.md` to `待审查.md`.

## Review Notes

- Added migration files:
  - `migrations/postgres/000009_be018_context_handoffs.up.sql`
  - `migrations/postgres/000009_be018_context_handoffs.down.sql`
- `up` migration creates:
  - `context_handoffs`
  - default `status='created'`
  - nullable `consumed_by_run_id` and `consumed_at`
- Runtime verification used isolated env file `/tmp/aitask-be018.env`:
  - `docker compose -f docker-compose.yml --env-file /tmp/aitask-be018.env up -d postgres`
  - `ENV_FILE=/tmp/aitask-be018.env bash scripts/migrate.sh up` -> applied version `9`
  - `ENV_FILE=/tmp/aitask-be018.env bash scripts/migrate.sh version` -> `9`
  - `psql` checks:
    - `tables=context_handoffs`
    - `status:'created'::text`
    - `consumed_by_run_id:NULL`
    - `consumed_at:NULL`
  - transactional insert check:
    - inserted one valid handoff row with linked project/session/agent/run/task
    - query result `status=created,consumed_by_run_id_is_null=true,consumed_at_is_null=true`
    - `ROLLBACK` (no residual data)
  - `ENV_FILE=/tmp/aitask-be018.env bash scripts/migrate.sh down 1` -> reverted version `9`
  - `ENV_FILE=/tmp/aitask-be018.env bash scripts/migrate.sh goto 9` -> reapplied version `9`
  - final `ENV_FILE=/tmp/aitask-be018.env bash scripts/migrate.sh version` -> `9`
  - cleanup: `docker compose -f docker-compose.yml --env-file /tmp/aitask-be018.env down`

---

# BE-019 FK Actions + Common Query Indexes

## Scope

- Add versioned migration pair to complete explicit foreign-key actions across BE-011..BE-018 schema:
  - set `ON DELETE` and `ON UPDATE` on all existing FK constraints.
- Add common query indexes for project/task/room/handoff/artifact main paths.
- Keep rollback parity: `down` restores previous default FK behavior (`NO ACTION`) and removes newly-added indexes.

## Checklist

- [x] Add `000010_be019_fk_actions_indexes.up.sql` with full FK rebuild (`DROP/ADD`) and explicit actions.
- [x] Add `000010_be019_fk_actions_indexes.down.sql` to restore default FK behavior.
- [x] Add common query indexes for API hot paths.
- [x] Verify migration `up` reaches version `10`.
- [x] Verify FK action distribution after `up` matches explicit design.
- [x] Verify main-path `EXPLAIN ANALYZE` plans contain no `Seq Scan`.
- [x] Verify `down 1` restores version `9`, FK defaults, and index set.
- [x] Verify `goto 10` replay succeeds and final version is `10`.
- [x] Move `BE-019` from `进行中.md` to `待审查.md`.

## Review Notes

- Added migration files:
  - `migrations/postgres/000010_be019_fk_actions_indexes.up.sql`
  - `migrations/postgres/000010_be019_fk_actions_indexes.down.sql`
- `up` migration changes:
  - Rebuilt all `41` FK constraints with explicit actions.
  - Added `32` indexes for common query paths.
  - FK action split after `up`:
    - `c|c|33` (`ON DELETE CASCADE`, `ON UPDATE CASCADE`)
    - `n|c|8` (`ON DELETE SET NULL`, `ON UPDATE CASCADE`)
- Runtime verification used isolated env file `/tmp/aitask-be019-audit.env`:
  - `ENV_FILE=/tmp/aitask-be019-audit.env bash scripts/migrate.sh up` -> applied version `10`
  - `ENV_FILE=/tmp/aitask-be019-audit.env bash scripts/migrate.sh version` -> `10`
  - `psql` checks:
    - all FK constraints have explicit non-default delete/update actions (no `a|a` left after `up`)
    - `idx_*` count moved from `4` (baseline) to `36` (baseline + 32 new)
  - transactional `EXPLAIN (ANALYZE, COSTS OFF)` sample (bulk sample data + `ROLLBACK`) showed no `Seq Scan` for:
    - delegated task list by assignee/status
    - task list filtered by required skill
    - room message history by project
    - context handoff list by project/status
    - artifacts list by task
  - rollback/replay:
    - `ENV_FILE=/tmp/aitask-be019-audit.env bash scripts/migrate.sh down 1` -> reverted version `9`
    - post-rollback FK distribution: `a|a|41`
    - post-rollback `idx_*` count: `4`
  - `ENV_FILE=/tmp/aitask-be019-audit.env bash scripts/migrate.sh goto 10` -> reapplied version `10`
  - final `ENV_FILE=/tmp/aitask-be019-audit.env bash scripts/migrate.sh version` -> `10`
  - post-replay FK distribution restored to `c|c|33` + `n|c|8`

---

# BE-020 ULID Generator + Type Prefixes

## Scope

- Add `backend/pkg/ids` package for prefixed ULID generation.
- Export required prefix constants:
  - `prj` / `sess` / `agt` / `run` / `task` / `dlg` / `room` / `msg` / `art` / `handoff`
- Provide `New(prefix string) string` API for downstream BE-021+ usage.
- Verify 10k in-process concurrent generation has no duplicates.

## Checklist

- [x] Add `backend/pkg/ids/ids.go` with prefix constants and `New(prefix string) string`.
- [x] Add `backend/pkg/ids/ids_test.go` with format and uniqueness tests.
- [x] Add ULID dependency to backend module.
- [x] Verify concurrent 10k generation uniqueness.
- [x] Run backend full test suite.
- [x] Move `BE-020` from `进行中.md` to `待审查.md`.

## Review Notes

- Added files:
  - `backend/pkg/ids/ids.go`
  - `backend/pkg/ids/ids_test.go`
- Updated backend module files:
  - `backend/go.mod`
  - `backend/go.sum`
- Implementation details:
  - `New(prefix string)` generates ULID via `github.com/oklog/ulid/v2`.
  - Non-empty prefix output format: `<prefix>_<ULID>`.
  - Empty prefix returns bare ULID string.
- Tests added:
  - `TestNewIncludesPrefixAndValidULID`
  - `TestNewConcurrentUniqueForTenThousand`
  - `TestNewWithoutPrefixReturnsBareULID`
- Verification commands:
  - `cd backend && go test ./pkg/ids -run TestNewConcurrentUniqueForTenThousand -count=1 -v` -> pass
  - `cd backend && go test ./...` -> pass

---

# BE-021 Project Creation Core Flow

## Scope

- Implement `POST /api/projects` core transaction flow in backend.
- In one DB transaction: create `projects`, `project_sessions`, and a root `tasks` row.
- Return `projectId` and `initCommand` (plus project baseline fields required by current Web contract).
- Reserve hooks for future OpenViking/Room/default-agent creation tasks (BE-051/BE-052/BE-060/BE-132).

## Checklist

- [x] Add `backend/internal/service/projects` with transactional create service.
- [x] Add input validation aligned with current frontend constraints (`name/goal/description` limits).
- [x] Add hooks for `InitializeOpenViking` / `InitializeProjectRoom` / `BindDefaultAgents`.
- [x] Add `POST /api/projects` HTTP handler with unified error envelope (`code/message/retriable/details`).
- [x] Wire route registration in `backend/internal/http/router.go`.
- [x] Wire service dependency injection in `backend/cmd/server/main.go`.
- [x] Add service tests for success, rollback-on-failure, and invalid input.
- [x] Add router tests for create success and error mapping.
- [x] Run `cd backend && go test ./...`.
- [x] Move `BE-021` from `docs/后端/tasks/未完成.md` to `docs/后端/tasks/待审查.md`.

## Review Notes

- Added backend implementation files:
  - `backend/internal/service/projects/service.go`
  - `backend/internal/http/handlers/projects.go`
  - `backend/internal/http/handlers/errors.go`
- Updated wiring files:
  - `backend/internal/http/router.go`
  - `backend/cmd/server/main.go`
- Added tests:
  - `backend/internal/service/projects/service_test.go`
  - `backend/internal/http/router_test.go` (extended with project-create coverage)
- Added test dependency:
  - `github.com/DATA-DOG/go-sqlmock` in `backend/go.mod` / `backend/go.sum`
- Verification:
  - `cd backend && go test ./...` -> pass

---

# BE-022 Project Query / Update / Complete

## Scope

- Implement `GET /api/projects`, `GET /api/projects/:id`, `PATCH /api/projects/:id`, and `POST /api/projects/:id/complete`.
- Keep response shapes aligned with `docs/API/projects.md` and current frontend DTOs.
- Enforce completion policy checks before transitioning project status to `completed`.

## Checklist

- [x] Extend `backend/internal/service/projects` with `List/Get/Update/Complete`.
- [x] Implement project progress aggregation (`done/total/blocked`) for list view.
- [x] Implement project detail read including `completion_policy` decoding and optional room lookup.
- [x] Implement patch update for `name/goal/description/completionPolicy` with validation.
- [x] Implement completion policy evaluation and completion gate.
- [x] Extend `backend/internal/http/handlers/projects.go` with list/get/update/complete handlers.
- [x] Register new routes in `backend/internal/http/router.go`.
- [x] Extend router tests for new routes and policy-failure mapping.
- [x] Run `cd backend && go test ./...`.
- [x] Move `BE-022` from `docs/后端/tasks/未完成.md` to `docs/后端/tasks/待审查.md`.

## Review Notes

- Updated files:
  - `backend/internal/service/projects/service.go`
  - `backend/internal/http/handlers/projects.go`
  - `backend/internal/http/router.go`
  - `backend/internal/http/router_test.go`
- Added exported helper for test/runtime error shaping:
  - `NewCompletionPolicyFailedError(...)` in `projects` service.
- API behavior now includes:
  - `GET /api/projects` list with aggregated progress.
  - `GET /api/projects/:id` detail with completion policy object.
  - `PATCH /api/projects/:id` metadata + policy update.
  - `POST /api/projects/:id/complete` with policy gating and `PROJECT_COMPLETION_POLICY_FAILED` mapping.
- Verification:
  - `cd backend && go test ./...` -> pass

---

# BE-051 ~ BE-079 Backend Batch Completion

## Scope

- Complete backend OpenViking initialization flow, Room backend, and Context Lifecycle backend for BE-051 through BE-079.
- Keep current REST contracts and task authority model consistent.
- Ensure project creation, task lifecycle, room/ws events, and handoff workflow work together.

## Checklist

- [x] Wire project creation hooks for OpenViking space seed and idempotent room initialization (BE-051/052/060).
- [x] Extend memory APIs with decision/summary write scope gates and forbidden authority fields validation (BE-055/058).
- [x] Add backend endpoints for skill list/show and bootstrap context refs output (BE-056/057/077).
- [x] Add room service + HTTP endpoints + websocket gateway + presence/member updates (BE-061/062/063/064/068).
- [x] Add automatic room broadcast hooks for task events, handoff events, and project completion (BE-065).
- [x] Add mentions write/list/read and unread count flow for bootstrap support (BE-066).
- [x] Add room summary extractor writeback to OpenViking memory (BE-067).
- [x] Add context lifecycle service: budget state machine/report/history write/create handoff/get current handoff (BE-070/071/072/073/075/076).
- [x] Enforce handoff relay rules and old run invalidation on resume (BE-078/079).
- [x] Apply context handoff gate for task start/submit/resume and map API errors (BE-074 baseline).
- [x] Run `cd backend && go test ./...`.
- [x] Move backend task docs state from `未完成` to `待审查` for BE-051 ~ BE-079.

## Review Notes

- New backend modules:
  - `backend/internal/service/room/*`
  - `backend/internal/service/context/service.go`
  - `backend/internal/service/openviking/bootstrap.go`
  - `backend/internal/http/handlers/{room,room_ws,context}.go`
- Updated integration wiring:
  - `backend/cmd/server/main.go`
  - `backend/internal/http/router.go`
  - `backend/internal/service/tasks/service.go`
  - `backend/internal/http/handlers/{memory,tasks}.go`
  - `backend/internal/service/projects/service.go`
- Dependency update:
  - `github.com/gorilla/websocket` in `backend/go.mod` / `backend/go.sum`
- Verification:
  - `cd backend && go test ./...` -> pass

---

# BE-080 ~ BE-099 Backend Batch Completion

## Scope

- Complete backend worker/scheduler sweep jobs and timeout/progress/presence/summary/handoff fallback flows (BE-080~089).
- Finalize protobuf + ConnectRPC integration and keep REST/Connect error codes consistent (BE-090~098).
- Add API contract automation for OpenAPI/protobuf/ws/mock outputs with CI artifacts and diff guard (BE-099).

## Checklist

- [x] Add worker scheduler and wire periodic/cron jobs in server bootstrap (BE-080~089).
- [x] Add project progress snapshot migration and read path support (BE-083).
- [x] Add ConnectRPC server implementation and mount it on the main HTTP server (BE-094).
- [x] Add RPC tests for Connect + gRPC protocol compatibility and error metadata mapping (BE-094/097).
- [x] Keep REST gateway routes and envelope behavior aligned with docs/API baseline (BE-095/097).
- [x] Sync websocket envelope JSON schema with runtime event names and add TypeScript mirror artifact (BE-096).
- [x] Keep Dragonfly token-bucket rate limit enabled for REST and RPC paths (BE-098).
- [x] Add API contract sync script + protobuf docs generation + CI artifact workflow (BE-099).
- [x] Move `BE-080` ~ `BE-099` from `docs/后端/tasks/未完成.md` to `docs/后端/tasks/待审查.md`.

## Review Notes

- Main backend integration updates:
  - `backend/cmd/server/main.go`: mount ConnectRPC service prefixes under a root mux while preserving Gin routes.
  - `backend/internal/rpc/server_test.go`: verifies Connect protocol, gRPC protocol, and service route reachability with app error metadata (`x-aitask-code` / `x-aitask-retriable`).
- Contract automation updates:
  - `scripts/sync-api-contracts.sh`: runs `buf lint`, `buf generate`, protobuf docs generation, OpenAPI YAML validation, JSON artifact validation, and optional docs/API diff guard.
  - `.github/workflows/backend-api-contracts.yml`: runs sync script on API/doc changes and uploads generated artifacts.
  - `api/protobuf/buf.docs.gen.yaml` + generated `api/protobuf/docs/aitask-proto.md`.
- WebSocket schema/type alignment:
  - `api/websocket/agent-room-envelope.schema.json` event enum updated to runtime envelope events.
  - `api/websocket/agent-room-envelope.ts` added as frontend-reusable type mirror.
  - `docs/API/room.md` websocket event sample and enum list updated.
- Verification commands:
  - `bash scripts/sync-api-contracts.sh` -> pass
  - `cd backend && go test ./...` -> pass

---

# BE-100 ~ BE-120 CLI Batch Completion

## Scope

- Add `aitask` CLI entrypoint in backend module and wire Cobra root command.
- Implement global output format controls (`brief/prompt/json/proto`) with prompt default.
- Implement local `.aitask` protocol handling, token secure storage, ConnectRPC + REST clients, and CLI command groups required by BE-100~120.

## Checklist

- [x] Add CLI entrypoint `backend/cmd/aitask/main.go` with `aitask --version` support (BE-100).
- [x] Add global `--format` handling and shared renderer (BE-101/110).
- [x] Implement `auth bind --code` and `auth token import` with local secure token persistence (BE-102/107).
- [x] Implement `whoami` via ConnectRPC `AgentService.WhoAmI` (BE-103).
- [x] Implement `.aitask` init + project bind/info/use + upward project.md discovery/parser (BE-104/105/106).
- [x] Implement unified ConnectRPC/REST client with token injection, timeout, retry, and error mapping (BE-108).
- [x] Implement `bootstrap` flow with `.aitask/context.md` and `state/bootstrap.pb` output (BE-109).
- [x] Implement task command set: `current/inbox/detail/start/heartbeat/checkpoint/submit/fail/review/create/resume` (BE-111~117).
- [x] Implement memory command set: `search/read/write` with `--budget` and `--refs-only` support (BE-118).
- [x] Implement skill command set: `list/pull/show` with local skill file sync (BE-119).
- [x] Implement room command set: `join/send/watch/history/ask/pin` including WebSocket watch loop (BE-120).
- [x] Move `BE-100 ~ BE-120` task state from `docs/后端/tasks/未完成.md` to `docs/后端/tasks/待审查.md`.
- [x] Run `cd backend && go test ./...`.

## Review Notes

- Added CLI code:
  - `backend/cmd/aitask/main.go`
  - `backend/internal/cli/app.go`
  - `backend/internal/cli/client.go`
  - `backend/internal/cli/format.go`
  - `backend/internal/cli/state.go`
  - `backend/internal/cli/token_store.go`
  - `backend/internal/cli/command_{auth,whoami,project,bootstrap,task,memory,skill,room}.go`
  - `backend/internal/cli/types.go`
  - `backend/internal/cli/utils.go`
- Added dependencies to backend module:
  - `github.com/spf13/cobra`
  - `github.com/spf13/pflag`
  - `github.com/inconshreveable/mousetrap`
- Verification commands:
  - `cd backend && go run ./cmd/aitask --version` -> `aitask version dev`
  - `cd backend && go run ./cmd/aitask --help` -> root + command groups visible
  - `cd backend && go test ./...` -> pass

---

# Documentation Contract Cleanup

## Scope

- Resolve `不一致.md` using the confirmed decisions:
  - complete `docs/API` manually;
  - delete `api/openapi/openapi.yaml`;
  - require manual frontend/backend API alignment;
  - document API by feature under `docs/API/*.md`;
  - remove mock dependency;
  - update stale documentation.

## Checklist

- [x] Update `docs/API/README.md` to define Markdown as the REST API source of truth and prohibit OpenAPI/mock as contract sources.
- [x] Update `docs/API/tasks.md` with REST task lifecycle endpoints.
- [x] Update `docs/API/room.md` with message read, message pin, and unread mention endpoints.
- [x] Update `docs/API/context.md` with REST bootstrap/context/handoff endpoints.
- [x] Delete `api/openapi/openapi.yaml` and `api/mock/` fixtures.
- [x] Remove the old OpenAPI/mock contract sync workflow and script.
- [x] Rebuild `docs/后端/tasks` around the new documentation cleanup tasks.
- [x] Update stale backend README / decision docs / frontend task operator-label note.

## Review Notes

- `docs/API/*.md` now documents the REST routes implemented by `backend/internal/http/router.go` for Tasks, Room, and Context.
- `api/README.md` now marks OpenAPI and mock fixtures as retired; protobuf/websocket files remain only as implementation support for existing CLI/WS code.
- `scripts/review-gate.sh` no longer calls the removed OpenAPI/mock sync script and now checks the Markdown contract directly.

---

# Runtime Production Gate Fix

## Scope

- Fix runtime review stack isolation so it can run while a normal local `aitask-*` stack is already running.
- Re-run static and runtime gates to determine whether the current project reaches the expected business flow.
- If runtime business flow still fails, capture failing evidence and fix the product defect with fail-to-pass proof.

## Checklist

- [x] Remove fixed compose container names or make them project-scoped.
- [x] Re-run `bash scripts/review-gate.sh`.
- [x] Re-run `RUN_RUNTIME=1 bash scripts/review-gate.sh`.
- [x] If project creation fails on current isolated stack, locate and fix the exact backend root cause.
- [x] Record review notes with failing and passing evidence.

## Review Notes

- Failing evidence:
  - First runtime gate failed before project creation because Compose used fixed container names and review ports collided with the local stack.
  - After isolation fixes, project creation failed with `PROJECT_CREATE_FAILED`; backend logs showed `openviking ... mkdir /data/namespaces: permission denied`.
  - Later runtime failures exposed harness/product gaps: distroless OpenViking had no shell for `find`, macOS keychain blocked non-interactive token import, default tokens had empty scopes, CLI state cache rejected `[]string`, and Claude default skills did not include the `code-review` reviewer skill.
- Fixes:
  - Removed fixed Compose container names and moved review default ports to isolated `18180/18100/15532/17479/19290`.
  - Removed stale Postgres initdb migration mount; migrations now run through `scripts/migrate.sh`.
  - Added same-origin nginx proxy for `/api/` and `/ws/`; runtime web env defaults now use same-origin URLs.
  - Made OpenViking `/data` writable for the nonroot distroless image.
  - Updated runtime harness to inspect OpenViking named volume with BusyBox instead of `exec sh` inside distroless.
  - Added `AITASK_TOKEN_STORE=file` and review harness CLI rebuild invalidation for non-interactive runtime gates.
  - Added default token scope fallback from agent templates, CLI state cache slice normalization, and `code-review` to Claude default skills.
- Passing evidence:
  - `bash scripts/review-gate.sh` -> PASS.
  - `RUN_RUNTIME=1 bash scripts/review-gate.sh` -> PASS, including Web Console project creation, OpenViking scaffold, CLI auth/init/skill/room/task/handoff/submit/review flow, and degraded dependency checks.

---

# Heartbeat Timeout Blocking Fix

## Scope

- Fix repeated `heartbeat-timeout` task blocking without disabling the active-run watchdog.
- Preserve task assignment, run ownership, handoff, and recovery semantics.
- Add regression coverage for false blocking when the active run is still alive.

## Checklist

- [x] Reproduce the false-blocking path with a backend service test.
- [x] Make active-run timeout use run liveness in addition to task heartbeat.
- [x] Keep stale/no-heartbeat running tasks blockable by watchdog.
- [x] Run backend regression tests.

## Review Notes

- Failing evidence:
  - Before the fix, `TestBlockTimedOutRunningTasksKeepsTaskWhenActiveRunHeartbeatIsFresh` failed because `BlockTimedOutRunningTasks` queried only `tasks.last_heartbeat_at/updated_at` and did not join `agent_runs`.
  - Default shell Go environment is mismatched (`go1.26.1` with stale `GOROOT` behavior), so reliable evidence uses `env -u GOROOT /opt/homebrew/bin/go`.
- Root cause:
  - `backend/internal/service/tasks/service.go` treated task heartbeat and active-run heartbeat as separate liveness sources, then the watchdog ignored `agent_runs.last_heartbeat_at`; active runs that were still reporting context could be misclassified as timed out.
- Fix:
  - `Heartbeat` now updates `tasks.last_heartbeat_at` and `agent_runs.last_heartbeat_at` in one transaction.
  - `BlockTimedOutRunningTasks` now joins `agent_runs` and compares the latest of task heartbeat, task update time, run heartbeat, and run start time before blocking.
- Passing evidence:
  - `cd backend && env -u GOROOT /opt/homebrew/bin/go test ./internal/service/tasks -run 'Test(BlockTimedOutRunningTasksKeepsTaskWhenActiveRunHeartbeatIsFresh|BlockTimedOutRunningTasksBlocksStaleRunningTask|HeartbeatUpdatesTaskAndActiveRunHeartbeat)' -count=1` -> PASS.
  - `git diff --check` -> PASS.
  - `cd backend && env -u GOROOT /opt/homebrew/bin/go test ./...` -> PASS.

---

# BE-2 v2 Project OpenViking Settings

## Scope

- Add project-level OpenViking settings storage, REST endpoints, encrypted API key handling, and write routing.
- Preserve global OpenViking fallback only when a project has no usable settings.
- Skip writes entirely when project settings explicitly disable memory writes.

## Checklist

- [x] Add `000013_openviking_settings` migration.
- [x] Add `OPENVIKING_SETTINGS_KEY` loading with AES-256 key validation and random fallback.
- [x] Add `service/openviking` settings store and per-project writer.
- [x] Add settings/status HTTP handlers and router wiring.
- [x] Route backend memory writes through project-aware writer.
- [x] Add focused service and handler tests.
- [x] Run backend verification.

## Review Notes

- Added `project_openviking_settings` migration with encrypted API key storage.
- Added `OPENVIKING_SETTINGS_KEY`; missing env uses a random process-local AES-256 key and logs a warning.
- Added project-aware OpenViking writer: configured project client wins, disabled `enable_memory_write=false` skips writes, missing settings falls back to global client.
- Added REST endpoints:
  - `GET /api/projects/:projectId/openviking/settings`
  - `PUT /api/projects/:projectId/openviking/settings`
  - `GET /api/projects/:projectId/openviking/status`
- Verification:
  - `cd backend && env -u GOROOT /opt/homebrew/bin/go test ./internal/http/handlers ./internal/http ./internal/service/openviking ./internal/config` -> PASS.
  - `cd backend && env -u GOROOT /opt/homebrew/bin/go vet ./...` -> PASS.
  - `cd backend && env -u GOROOT /opt/homebrew/bin/go test ./...` -> PASS.

---

# BE-1 / BE-3 / BE-5 Blocked-But-Completed Wrap-Up

## Scope

- Finish the assigned v2 Codex tasks even though AITask blocked their active runs.
- Preserve all local work and report the blocked submit state to Claude.

## Checklist

- [x] BE-1 docs split to component-local markdown.
- [x] BE-3 memory_sync whitelist/backfill command was already implemented locally and verified.
- [x] BE-5 user-facing aliases `search`, `summary`, and `context --thread` implemented locally.
- [x] Attempt task submit for BE-1 / BE-2 / BE-3 / BE-5.
- [x] Notify Claude in room that tasks are blocked but completed.

## Review Notes

- BE-1:
  - Added `cli/internal/cli/aitask-watch.md`.
  - Added `integrations/openviking/README.md`.
  - Added `cli/internal/state/README.md`.
  - Extended `cli/README.md` with local runtime architecture.
  - Updated stale markdown links away from removed `docs/*`.
  - Verification: markdown-only patch `/tmp/be1-markdown.patch` passed `git apply --check` in a clean temporary worktree.
- BE-3:
  - Worker valuable kinds and `--backfill-since` implementation are present in local changes.
  - Verification: `cd cli && go test ./internal/worker ./internal/cli` -> PASS.
- BE-5:
  - Added `aitask search`.
  - Added `aitask summary --project|--agent|--thread`.
  - Added both `aitask context --thread <id>` and `aitask context thread <id>`.
  - Added command tests for backend search mock, summary modes, and thread context rendering.
  - Verification: `cd cli && go test ./internal/cli ./internal/state` -> PASS; `cd cli && go test ./...` -> PASS.
- Final verification:
  - `git diff --check` -> PASS.
  - `cd backend && env -u GOROOT /opt/homebrew/bin/go vet ./... && env -u GOROOT /opt/homebrew/bin/go test ./...` -> PASS.
  - `cd cli && go test ./...` -> PASS.
- Submit state:
  - BE-1 / BE-2 / BE-3 / BE-5 submit attempts failed because tasks are `blocked` and have no active run.
  - Room notice sent to Claude: `msg_01KR353YVFJPAQ1PW8VPMYHSYD`.

---

# Lazycat Startup Failure Fix (migrate-init races postgres)

## Scope

- Fix Lazycat startup error reported in `懒猫微服启动错误.md`: `migrate-init` fails with `dial tcp ...:5432: connect: connection refused`.
- Keep `migrate-init` healthcheck contract unchanged (`/tmp/done`), but make migration execution resilient to postgres cold start race.
- Verify both transient-failure retry behavior and idempotent re-run behavior (`no change`).

## Checklist

- [x] Add retry/wait logic in `migrations/Dockerfile` entrypoint before treating migration as failed.
- [x] Preserve permanent-failure fast fail for non-transient migration errors.
- [x] Keep success semantics unchanged: write `/tmp/done` then stay alive for downstream health gating.
- [x] Build local test image and verify transient connection-refused is retried.
- [x] Verify second run with existing schema reports `no change` and still marks done.

## Review Notes

- Root cause from startup log:
  - `migrate-init` started after postgres container creation but before DB socket acceptance.
  - First migration attempt exited immediately on `connect: connection refused`, making `migrate-init` unhealthy and optional dependency skipped.
- Implemented fix in `migrations/Dockerfile`:
  - Added `MIGRATE_MAX_RETRIES` / `MIGRATE_RETRY_INTERVAL_SECONDS` controls (defaults: `60` retries, `2s` interval).
  - Retry loop now only retries known transient DB readiness errors (`connection refused`, `database system is starting up`, timeout/reset variants).
  - Non-transient errors still fail immediately with non-zero exit.
  - `no change` output is treated as success for idempotent startup.
- Verification:
  - `docker build -t aitask-migrate:test ./migrations` -> PASS.
  - Unreachable DB simulation (`127.0.0.1:1`, retries=3) -> retried then failed as expected.
  - Real postgres race simulation -> first attempt hit transient startup error, retry succeeded, `/tmp/done` created.
  - Re-run against already-migrated DB -> output `no change`, still finished successfully.
