package openviking

import (
	"context"
	"fmt"
	"strings"
)

const templateVersion = "aitask-template-v1"
const bootstrapSeedEntryLimit = 1

type ProjectSeedInput struct {
	ProjectName        string
	ProjectGoal        string
	ProjectDescription string
}

// SeedProjectSpace writes the initial brief/skills/memory entries into the
// project's OpenViking space via the supplied writer. Pass a *ProjectAwareWriter
// so writes honor system OpenViking settings (URL/API key/toggles); pass a
// *Client to bypass settings and write directly to the global endpoint.
func SeedProjectSpace(ctx context.Context, writer MemoryClient, projectID string, input ProjectSeedInput) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return &Error{Op: "initialize_project_space", Kind: ErrorKindBadRequest, Err: fmt.Errorf("project id cannot be empty")}
	}
	if writer == nil {
		return &Error{Op: "initialize_project_space", Kind: ErrorKindUnavailable, Err: fmt.Errorf("openviking writer is nil")}
	}
	// OpenViking applies per-path locks and can return "resource is busy" when
	// many bootstrap writes happen in a burst; keep create-project path reliable
	// by seeding the minimal critical document only.
	entries := buildSeedEntries(input)
	if len(entries) > bootstrapSeedEntryLimit {
		entries = entries[:bootstrapSeedEntryLimit]
	}
	for _, entry := range entries {
		if _, err := writer.Write(ctx, projectID, entry); err != nil {
			return err
		}
	}
	return nil
}

func buildSeedEntries(input ProjectSeedInput) []WriteInput {
	name := strings.TrimSpace(input.ProjectName)
	goal := strings.TrimSpace(input.ProjectGoal)
	description := strings.TrimSpace(input.ProjectDescription)
	if name == "" {
		name = "Unnamed Project"
	}
	if goal == "" {
		goal = "Define clear delivery goals."
	}
	if description == "" {
		description = "No description provided."
	}

	projectGoal := fmt.Sprintf(`# Project Goal

- Template version: %s
- Project: %s

## Goal

%s

## Description

%s
`, templateVersion, name, goal, description)

	productScope := fmt.Sprintf(`# Product Scope

- Template version: %s
- Project: %s

## Included

- Persistent task orchestration for Claude Code, Codex, and Gemini.
- PostgreSQL task authority, DragonflyDB presence/cache, and OpenViking memory sync.
- Web Console, CLI bootstrap, delegated task execution, and review workflow.
`, templateVersion, name)

	constraints := fmt.Sprintf(`# Constraints

- Template version: %s
- Task authority is backend database state; do not treat memory as source of truth.
- Active run ownership is authoritative on backend.
- Agent identity must come from token, never from request body.
- Record decisions and summaries in memory as references, not authority.
`, templateVersion)

	acceptanceCriteria := fmt.Sprintf(`# Acceptance Criteria

- Template version: %s
- Project creation returns project/session/room/OpenViking bootstrap metadata.
- Delegated tasks can only be started and submitted by the assigned agent run.
- OpenViking stores summaries, skills, handoffs, and long-lived references.
- Room events and task events stay consistent with backend authority.
`, templateVersion)

	projectCoordinator := fmt.Sprintf(`# Skill: project-coordinator

Template version: %s

You are responsible for project orchestration.

## Responsibilities

- Break down project goals into delegated tasks.
- Keep task ownership explicit by assignee and active run.
- Request review before marking cross-cutting work complete.
- Keep room messages focused on blockers and decisions.
`, templateVersion)

	backendImplementation := fmt.Sprintf(`# Skill: backend-implementation

Template version: %s

You are responsible for backend implementation.

## Responsibilities

- Implement API and service changes with tests.
- Keep project/task authority in backend database.
- Use memory as supporting context, not source of truth.
- Report checkpoints and submit artifacts with clear summaries.
`, templateVersion)

	documentGeneration := fmt.Sprintf(`# Skill: document-generation

Template version: %s

You are responsible for structured writing deliverables.

## Responsibilities

- Produce requirement summaries, API docs, and release notes.
- Keep terminology aligned with backend authority and CLI behavior.
- Convert large execution details into concise summaries and handoffs.
`, templateVersion)

	codeReview := fmt.Sprintf(`# Skill: code-review

Template version: %s

You are responsible for code review.

## Review Focus

- Behavioral regressions and correctness risks.
- Missing tests and edge-case handling.
- Contract compatibility for API and schema changes.
- Security or permission bypass vectors.
`, templateVersion)

	finalAcceptance := fmt.Sprintf(`# Skill: final-acceptance

Template version: %s

You are responsible for release validation.

## Review Focus

- Verify Docker Compose, CLI, OpenViking, and Web Console flows in a real environment.
- Collect repeatable evidence for review tasks and release gates.
- Block release when behavior diverges from docs or contract artifacts.
`, templateVersion)

	entries := []WriteInput{
		{Target: "brief", Title: "project-goal", Content: projectGoal},
		{Target: "brief", Title: "product-scope", Content: productScope},
		{Target: "brief", Title: "constraints", Content: constraints},
		{Target: "brief", Title: "acceptance-criteria", Content: acceptanceCriteria},
		{Target: "skills", Title: "project-coordinator", Content: projectCoordinator},
		{Target: "skills", Title: "backend-implementation", Content: backendImplementation},
		{Target: "skills", Title: "document-generation", Content: documentGeneration},
		{Target: "skills", Title: "code-review", Content: codeReview},
		{Target: "skills", Title: "final-acceptance", Content: finalAcceptance},
		{Target: "decision", Title: "README", Content: "# Decisions\n\nLong-lived architectural decisions live here."},
		{Target: "summary", Title: "current-project-summary", Content: "# Current Project Summary\n\nBootstrap not started yet."},
		{Target: "memory", Title: "agent-experience/README", Content: "# Agent Experience\n\nKeep agent-specific working notes and lessons."},
		{Target: "memory", Title: "room/README", Content: "# Room Memory\n\nKeep high-value room summaries, not raw chat transcripts."},
		{Target: "handoff", Title: "README", Content: "# Handoffs\n\nStore durable handoff snapshots for resumed runs."},
		{Target: "memory", Title: "mistakes/README", Content: "# Mistakes\n\nCapture recurring mistakes and their fixes."},
		{Target: "resources", Title: "api/README", Content: "# API Resources\n\nStore reusable API references and summaries."},
		{Target: "resources", Title: "database/README", Content: "# Database Resources\n\nStore schema notes and migration references."},
		{Target: "resources", Title: "cli/README", Content: "# CLI Resources\n\nStore CLI examples and command references."},
		{Target: "resources", Title: "frontend/README", Content: "# Frontend Resources\n\nStore UI flows and frontend implementation notes."},
		{Target: "tasks", Title: "task-tree-summary", Content: "# Task Tree Summary\n\nRoot task created; task tree not expanded yet."},
		{Target: "tasks", Title: "active-tasks", Content: "# Active Tasks\n\nNo active tasks yet."},
		{Target: "tasks", Title: "blocked-tasks", Content: "# Blocked Tasks\n\nNo blocked tasks yet."},
		{Target: "tasks", Title: "completed-tasks-summary", Content: "# Completed Tasks Summary\n\nNo completed tasks yet."},
		{Target: "sessions", Title: "README", Content: "# Sessions\n\nStore session snapshots and final summaries per session."},
	}

	// Seed writes are bootstrap scaffolding; keep creation fast and let
	// OpenViking semantic/vector indexing run asynchronously.
	for i := range entries {
		entries[i].AutoSync = true
	}
	return entries
}
