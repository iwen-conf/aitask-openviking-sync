package sessions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrSessionNotFound = errors.New("active session not found")
)

type InvalidInputError struct {
	details map[string]string
}

func (e *InvalidInputError) Error() string {
	return "invalid session input"
}

func (e *InvalidInputError) Details() map[string]string {
	if len(e.details) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(e.details))
	for k, v := range e.details {
		out[k] = v
	}
	return out
}

func newInvalidInputError(details map[string]string) error {
	copyMap := make(map[string]string, len(details))
	for k, v := range details {
		copyMap[k] = v
	}
	return &InvalidInputError{details: copyMap}
}

type Service struct {
	db *sql.DB
}

type ActiveSession struct {
	SessionID      string
	ProjectID      string
	Status         string
	CurrentPhase   string
	ContextSummary string
	UpdatedAt      time.Time
}

func New(db *sql.DB) (*Service, error) {
	if db == nil {
		return nil, errors.New("sessions service requires database handle")
	}
	return &Service{db: db}, nil
}

func (s *Service) GetActive(ctx context.Context, projectID string) (ActiveSession, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ActiveSession{}, newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}

	var session ActiveSession
	var summary sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT
			ps.id,
			ps.project_id,
			ps.status,
			COALESCE(ps.current_phase, ''),
			ps.context_summary,
			ps.updated_at
		FROM projects p
		JOIN project_sessions ps ON ps.id = p.active_session_id
		WHERE p.id = $1
	`, projectID).Scan(
		&session.SessionID,
		&session.ProjectID,
		&session.Status,
		&session.CurrentPhase,
		&summary,
		&session.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		if exists, existsErr := s.projectExists(ctx, projectID); existsErr != nil {
			return ActiveSession{}, existsErr
		} else if !exists {
			return ActiveSession{}, ErrProjectNotFound
		}
		return ActiveSession{}, ErrSessionNotFound
	}
	if err != nil {
		return ActiveSession{}, fmt.Errorf("query active session failed: %w", err)
	}

	if summary.Valid {
		session.ContextSummary = summary.String
	}
	return session, nil
}

func (s *Service) SwitchPhase(ctx context.Context, projectID string, phase string) (ActiveSession, error) {
	projectID = strings.TrimSpace(projectID)
	phase = strings.TrimSpace(phase)
	if projectID == "" || phase == "" {
		details := map[string]string{}
		if projectID == "" {
			details["projectId"] = "cannot be empty"
		}
		if phase == "" {
			details["phase"] = "cannot be empty"
		}
		return ActiveSession{}, newInvalidInputError(details)
	}

	sessionID, err := s.activeSessionID(ctx, projectID)
	if err != nil {
		return ActiveSession{}, err
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE project_sessions
		SET current_phase = $2,
			version = version + 1,
			updated_at = NOW()
		WHERE id = $1
	`, sessionID, phase); err != nil {
		return ActiveSession{}, fmt.Errorf("switch session phase failed: %w", err)
	}

	return s.GetActive(ctx, projectID)
}

func (s *Service) UpdateSummary(ctx context.Context, projectID string, summary string) (ActiveSession, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ActiveSession{}, newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}

	sessionID, err := s.activeSessionID(ctx, projectID)
	if err != nil {
		return ActiveSession{}, err
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE project_sessions
		SET context_summary = $2,
			version = version + 1,
			updated_at = NOW()
		WHERE id = $1
	`, sessionID, nullableSummary(summary)); err != nil {
		return ActiveSession{}, fmt.Errorf("update session summary failed: %w", err)
	}

	return s.GetActive(ctx, projectID)
}

func (s *Service) activeSessionID(ctx context.Context, projectID string) (string, error) {
	var sessionID sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT active_session_id FROM projects WHERE id = $1`, projectID).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrProjectNotFound
	}
	if err != nil {
		return "", fmt.Errorf("query project active session id failed: %w", err)
	}
	if !sessionID.Valid || strings.TrimSpace(sessionID.String) == "" {
		return "", ErrSessionNotFound
	}
	return sessionID.String, nil
}

func (s *Service) projectExists(ctx context.Context, projectID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1)`, projectID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check project exists failed: %w", err)
	}
	return exists, nil
}

func nullableSummary(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return value
}
