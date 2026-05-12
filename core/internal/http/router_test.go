package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	agentsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/agents"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/health"
	projectsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/projects"
	tasksvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/tasks"
)

func newTestRouter(now time.Time, readiness *health.ReadinessService) *gin.Engine {
	return newTestRouterWithProjects(now, readiness, nil)
}

type projectCreatorStub struct {
	createOutput    projectsvc.CreateProjectOutput
	createErr       error
	lastCreateInput projectsvc.CreateProjectInput

	listOutput projectsvc.ListProjectsOutput
	listErr    error

	getOutput    projectsvc.ProjectDetail
	getErr       error
	lastGetID    string
	updateOutput projectsvc.UpdateProjectOutput
	updateErr    error
	lastUpdateID string
	lastUpdate   projectsvc.UpdateProjectInput

	completeOutput projectsvc.CompleteProjectOutput
	completeErr    error
	lastCompleteID string
	lastComplete   projectsvc.CompleteProjectInput

	archiveOutput projectsvc.ArchiveProjectOutput
	archiveErr    error
	lastArchiveID string
	lastArchive   projectsvc.ArchiveProjectInput
}

func (s *projectCreatorStub) Create(_ context.Context, input projectsvc.CreateProjectInput) (projectsvc.CreateProjectOutput, error) {
	s.lastCreateInput = input
	return s.createOutput, s.createErr
}

func (s *projectCreatorStub) List(_ context.Context) (projectsvc.ListProjectsOutput, error) {
	return s.listOutput, s.listErr
}

func (s *projectCreatorStub) Get(_ context.Context, projectID string) (projectsvc.ProjectDetail, error) {
	s.lastGetID = projectID
	return s.getOutput, s.getErr
}

func (s *projectCreatorStub) Update(_ context.Context, projectID string, input projectsvc.UpdateProjectInput) (projectsvc.UpdateProjectOutput, error) {
	s.lastUpdateID = projectID
	s.lastUpdate = input
	return s.updateOutput, s.updateErr
}

func (s *projectCreatorStub) Complete(_ context.Context, projectID string, input projectsvc.CompleteProjectInput) (projectsvc.CompleteProjectOutput, error) {
	s.lastCompleteID = projectID
	s.lastComplete = input
	return s.completeOutput, s.completeErr
}

func (s *projectCreatorStub) Archive(_ context.Context, projectID string, input projectsvc.ArchiveProjectInput) (projectsvc.ArchiveProjectOutput, error) {
	s.lastArchiveID = projectID
	s.lastArchive = input
	return s.archiveOutput, s.archiveErr
}

func newTestRouterWithProjects(
	now time.Time,
	readiness *health.ReadinessService,
	projectsCreator *projectCreatorStub,
) *gin.Engine {
	return newTestRouterWithServices(now, readiness, projectsCreator, nil, nil)
}

func newTestRouterWithServices(
	now time.Time,
	readiness *health.ReadinessService,
	projectsCreator *projectCreatorStub,
	agentsService *agentsvc.Service,
	tasksService *tasksvc.Service,
) *gin.Engine {
	return NewRouter(RouterDeps{
		Health: health.New(health.Options{
			ServiceName: "aitask-backend",
			Version:     "test",
			Now:         func() time.Time { return now },
		}),
		Readiness:            readiness,
		Projects:             projectsCreator,
		Agents:               agentsService,
		Tasks:                tasksService,
		ConsoleOperatorLabel: "console-operator",
		OpenVikingNamespace:  "aitask",
	})
}

func newTestAgentsService(t *testing.T, now time.Time) (*agentsvc.Service, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	service, err := agentsvc.New(agentsvc.Options{
		DB:          db,
		TokenSecret: "test-secret",
		Now:         func() time.Time { return now },
	})
	if err != nil {
		_ = db.Close()
		t.Fatalf("agents.New() error = %v", err)
	}

	cleanup := func() {
		_ = db.Close()
	}
	return service, mock, cleanup
}

func newTestTasksService(t *testing.T) (*tasksvc.Service, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	service, err := tasksvc.New(tasksvc.Options{
		DB:                   db,
		ConsoleOperatorLabel: "console-operator",
	})
	if err != nil {
		_ = db.Close()
		t.Fatalf("tasks.New() error = %v", err)
	}

	cleanup := func() {
		_ = db.Close()
	}
	return service, mock, cleanup
}

func TestHealthz(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	router := newTestRouter(now, health.NewReadiness(health.ReadinessOptions{}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body["status"], "ok"; got != want {
		t.Fatalf("status field = %q, want %q", got, want)
	}
	if got, want := body["time"], "2026-04-30T12:00:00Z"; got != want {
		t.Fatalf("time field = %q, want %q", got, want)
	}
}

func TestReadyzReady(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newTestRouter(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{
			Dependencies: []health.Dependency{
				{
					Name:     "postgres",
					Critical: true,
					Check: func(context.Context) error {
						return nil
					},
				},
				{
					Name:     "dragonfly",
					Critical: true,
					Check: func(context.Context) error {
						return nil
					},
				},
				{
					Name:     "openviking",
					Critical: false,
					Check: func(context.Context) error {
						return nil
					},
				},
			},
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Status       string            `json:"status"`
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Status, health.ReadyStatusReady; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
}

func TestReadyzDegradedWhenOpenVikingUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newTestRouter(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{
			Dependencies: []health.Dependency{
				{
					Name:     "postgres",
					Critical: true,
					Check: func(context.Context) error {
						return nil
					},
				},
				{
					Name:     "dragonfly",
					Critical: true,
					Check: func(context.Context) error {
						return nil
					},
				},
				{
					Name:     "openviking",
					Critical: false,
					Check: func(context.Context) error {
						return errors.New("dial tcp timeout")
					},
				},
			},
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Status       string            `json:"status"`
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Status, health.ReadyStatusDegraded; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := body.Dependencies["openviking"], health.DependencyStatusUnavailable; got != want {
		t.Fatalf("dependencies[openviking] = %q, want %q", got, want)
	}
}

func TestReadyzNotReadyWhenCriticalDependencyUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newTestRouter(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{
			Dependencies: []health.Dependency{
				{
					Name:     "postgres",
					Critical: true,
					Check: func(context.Context) error {
						return errors.New("connection refused")
					},
				},
				{
					Name:     "dragonfly",
					Critical: true,
					Check: func(context.Context) error {
						return nil
					},
				},
				{
					Name:     "openviking",
					Critical: false,
					Check: func(context.Context) error {
						return nil
					},
				},
			},
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	var body struct {
		Status       string            `json:"status"`
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Status, health.ReadyStatusNotReady; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := body.Dependencies["postgres"], health.DependencyStatusUnavailable; got != want {
		t.Fatalf("dependencies[postgres] = %q, want %q", got, want)
	}
}

func TestCreateProjectSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub := &projectCreatorStub{
		createOutput: projectsvc.CreateProjectOutput{
			ProjectID:       "prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN",
			Name:            "AgentTaskSystem",
			Status:          "active",
			ActiveSessionID: "sess_01HX9ZK7Q6T3V5M2P8DAW4R9CP",
			OpenVikingRoot:  "viking://aitask/projects/prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN",
			RoomID:          "room_01HX9ZK7Q6T3V5M2P8DAW4R9CQ",
			InitCommand:     "aitask init --project prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN",
		},
	}
	router := newTestRouterWithProjects(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		stub,
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/projects",
		bytes.NewBufferString(`{"name":"AgentTaskSystem","goal":"Build a persistent AI Agent project orchestration system","description":"Persistent task orchestration"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body["projectId"], "prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN"; got != want {
		t.Fatalf("projectId = %v, want %v", got, want)
	}
	if got, want := body["initCommand"], "aitask init --project prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN"; got != want {
		t.Fatalf("initCommand = %v, want %v", got, want)
	}

	if got, want := stub.lastCreateInput.Name, "AgentTaskSystem"; got != want {
		t.Fatalf("service input name = %q, want %q", got, want)
	}
}

func TestCreateProjectInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newTestRouterWithProjects(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		&projectCreatorStub{},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewBufferString(`{"name":`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var body struct {
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		Retriable bool           `json:"retriable"`
		Details   map[string]any `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "INVALID_REQUEST"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if body.Retriable {
		t.Fatalf("retriable = true, want false")
	}
}

func TestCreateProjectInvalidArgumentError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newTestRouterWithProjects(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		&projectCreatorStub{
			createErr: projectsvc.NewInvalidInputError(map[string]string{
				"name": "must be between 2 and 80 characters",
			}),
		},
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/projects",
		bytes.NewBufferString(`{"name":"A","goal":"x"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var body struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "INVALID_ARGUMENT"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if _, ok := body.Details["name"]; !ok {
		t.Fatalf("details[name] missing: %+v", body.Details)
	}
}

func TestCreateProjectInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newTestRouterWithProjects(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		&projectCreatorStub{createErr: projectsvc.ErrCreateProjectFailed},
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/projects",
		bytes.NewBufferString(`{"name":"AgentTaskSystem","goal":"Build a persistent AI Agent project orchestration system"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	var body struct {
		Code      string `json:"code"`
		Retriable bool   `json:"retriable"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "PROJECT_CREATE_FAILED"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if !body.Retriable {
		t.Fatalf("retriable = false, want true")
	}
}

func TestListProjectsSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newTestRouterWithProjects(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		&projectCreatorStub{
			listOutput: projectsvc.ListProjectsOutput{
				Items: []projectsvc.ProjectListItem{
					{
						ProjectID: "prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN",
						Name:      "AgentTaskSystem",
						Status:    "active",
						Progress: projectsvc.ProjectProgress{
							Done:    8,
							Total:   24,
							Blocked: 1,
						},
						UpdatedAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Items []struct {
			ProjectID string `json:"projectId"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(body.Items))
	}
	if got, want := body.Items[0].ProjectID, "prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN"; got != want {
		t.Fatalf("projectId = %q, want %q", got, want)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newTestRouterWithProjects(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		&projectCreatorStub{getErr: projectsvc.ErrProjectNotFound},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/projects/prj_missing", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "PROJECT_NOT_FOUND"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
}

func TestUpdateProjectSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	stub := &projectCreatorStub{
		updateOutput: projectsvc.UpdateProjectOutput{
			ProjectID: "prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN",
			Name:      "AgentTaskSystem v2",
			Status:    "active",
			UpdatedAt: time.Date(2026, 4, 30, 12, 10, 0, 0, time.UTC),
		},
	}
	router := newTestRouterWithProjects(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		stub,
	)

	req := httptest.NewRequest(http.MethodPatch, "/api/projects/prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN", bytes.NewBufferString(`{"name":"AgentTaskSystem v2"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if got, want := stub.lastUpdateID, "prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN"; got != want {
		t.Fatalf("update project id = %q, want %q", got, want)
	}
	if stub.lastUpdate.Name == nil || *stub.lastUpdate.Name != "AgentTaskSystem v2" {
		t.Fatalf("update input name = %+v, want AgentTaskSystem v2", stub.lastUpdate.Name)
	}
}

func TestCompleteProjectPolicyFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newTestRouterWithProjects(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		&projectCreatorStub{
			completeErr: projectsvc.NewCompletionPolicyFailedError([]projectsvc.CompletionPolicyResultItem{
				{
					Code:    "REQUIRED_TASKS_NOT_DONE",
					Message: "1 required tasks are not done",
				},
			}),
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN/complete", bytes.NewBufferString(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}

	var body struct {
		Code    string `json:"code"`
		Details struct {
			FailedItems []map[string]any `json:"failedItems"`
		} `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "PROJECT_COMPLETION_POLICY_FAILED"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if len(body.Details.FailedItems) != 1 {
		t.Fatalf("failedItems length = %d, want 1", len(body.Details.FailedItems))
	}
}

func TestArchiveProjectSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updatedAt := time.Date(2026, 4, 30, 12, 30, 0, 0, time.UTC)
	stub := &projectCreatorStub{
		archiveOutput: projectsvc.ArchiveProjectOutput{
			ProjectID: "prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN",
			Status:    "archived",
			UpdatedAt: updatedAt,
		},
	}
	router := newTestRouterWithProjects(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		stub,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN/archive", bytes.NewBufferString(`{"confirm":true,"reason":"release finished"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got, want := stub.lastArchiveID, "prj_01HX9ZK7Q6T3V5M2P8DAW4R9CN"; got != want {
		t.Fatalf("archive project id = %q, want %q", got, want)
	}
	if !stub.lastArchive.Confirm {
		t.Fatalf("archive confirm = false, want true")
	}

	var body struct {
		ProjectID string    `json:"projectId"`
		Status    string    `json:"status"`
		UpdatedAt time.Time `json:"updatedAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Status, "archived"; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if !body.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updatedAt = %v, want %v", body.UpdatedAt, updatedAt)
	}
}

func TestAPIRejectsAgentTokenExpired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	agentsService, agentsMock, cleanup := newTestAgentsService(t, now)
	defer cleanup()

	agentsMock.ExpectQuery("SELECT[\\s\\S]*FROM agent_tokens t[\\s\\S]*WHERE t.token_hash = \\$1").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "agent_id", "scopes", "expires_at", "revoked_at", "agent_type", "role", "status",
		}).AddRow(
			"tok_1", "agt_1", "[\"task:start\"]", now.Add(-1*time.Minute), nil, "codex", "worker", "active",
		))

	router := newTestRouterWithServices(
		now,
		health.NewReadiness(health.ReadinessOptions{}),
		&projectCreatorStub{listOutput: projectsvc.ListProjectsOutput{Items: []projectsvc.ProjectListItem{}}},
		agentsService,
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer aitask_at_tok.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "AGENT_TOKEN_EXPIRED"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}

	if err := agentsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("agents expectations were not met: %v", err)
	}
}

func TestAPIRejectsAgentTokenInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	agentsService, agentsMock, cleanup := newTestAgentsService(t, now)
	defer cleanup()

	agentsMock.ExpectQuery("SELECT[\\s\\S]*FROM agent_tokens t[\\s\\S]*WHERE t.token_hash = \\$1").
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)

	router := newTestRouterWithServices(
		now,
		health.NewReadiness(health.ReadinessOptions{}),
		&projectCreatorStub{listOutput: projectsvc.ListProjectsOutput{Items: []projectsvc.ProjectListItem{}}},
		agentsService,
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer aitask_at_tok.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "AGENT_TOKEN_INVALID"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}

	if err := agentsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("agents expectations were not met: %v", err)
	}
}

func TestProjectScopedTasksRejectCrossProjectAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	agentsService, agentsMock, cleanupAgents := newTestAgentsService(t, now)
	defer cleanupAgents()
	tasksService, tasksMock, cleanupTasks := newTestTasksService(t)
	defer cleanupTasks()

	agentsMock.ExpectQuery("SELECT[\\s\\S]*FROM agent_tokens t[\\s\\S]*WHERE t.token_hash = \\$1").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "agent_id", "scopes", "expires_at", "revoked_at", "agent_type", "role", "status",
		}).AddRow(
			"tok_1", "agt_1", "[\"task:list\"]", now.Add(30*time.Minute), nil, "codex", "worker", "active",
		))
	agentsMock.ExpectQuery("SELECT project_id FROM agent_project_bindings[\\s\\S]*").
		WithArgs("agt_1").
		WillReturnRows(sqlmock.NewRows([]string{"project_id"}).AddRow("prj_allowed"))

	router := newTestRouterWithServices(
		now,
		health.NewReadiness(health.ReadinessOptions{}),
		nil,
		agentsService,
		tasksService,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/projects/prj_denied/tasks", nil)
	req.Header.Set("Authorization", "Bearer aitask_at_tok.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "PROJECT_ACCESS_DENIED"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}

	if err := agentsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("agents expectations were not met: %v", err)
	}
	if err := tasksMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("tasks expectations were not met: %v", err)
	}
}

func TestCreateTaskReturnsInvalidArgumentForBadInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tasksService, tasksMock, cleanup := newTestTasksService(t)
	defer cleanup()

	router := newTestRouterWithServices(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		nil,
		nil,
		tasksService,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/prj_1/tasks", bytes.NewBufferString(`{"title":"x","description":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "INVALID_ARGUMENT"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}

	if err := tasksMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("tasks expectations were not met: %v", err)
	}
}

func TestDelegateTaskReturnsInvalidArgumentForMissingAssignee(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tasksService, tasksMock, cleanup := newTestTasksService(t)
	defer cleanup()

	router := newTestRouterWithServices(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		nil,
		nil,
		tasksService,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/prj_1/tasks/task_1/delegate", bytes.NewBufferString(`{"agentId":"","agentType":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "INVALID_ARGUMENT"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}

	if err := tasksMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("tasks expectations were not met: %v", err)
	}
}

func TestStartTaskRequiresAssignedAgentIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tasksService, tasksMock, cleanup := newTestTasksService(t)
	defer cleanup()

	router := newTestRouterWithServices(
		time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		health.NewReadiness(health.ReadinessOptions{}),
		nil,
		nil,
		tasksService,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/prj_1/tasks/task_1/start", bytes.NewBufferString(`{"runId":"run_1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "TASK_NOT_ASSIGNED_TO_CURRENT_AGENT"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}

	if err := tasksMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("tasks expectations were not met: %v", err)
	}
}

func TestTaskScopeDeniedWithoutStartScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	agentsService, agentsMock, cleanupAgents := newTestAgentsService(t, now)
	defer cleanupAgents()
	tasksService, tasksMock, cleanupTasks := newTestTasksService(t)
	defer cleanupTasks()

	agentsMock.ExpectQuery("SELECT[\\s\\S]*FROM agent_tokens t[\\s\\S]*WHERE t.token_hash = \\$1").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "agent_id", "scopes", "expires_at", "revoked_at", "agent_type", "role", "status",
		}).AddRow(
			"tok_1", "agt_1", "[\"task:read:own\"]", now.Add(30*time.Minute), nil, "codex", "worker", "active",
		))
	agentsMock.ExpectQuery("SELECT project_id FROM agent_project_bindings[\\s\\S]*").
		WithArgs("agt_1").
		WillReturnRows(sqlmock.NewRows([]string{"project_id"}).AddRow("prj_1"))

	router := newTestRouterWithServices(
		now,
		health.NewReadiness(health.ReadinessOptions{}),
		nil,
		agentsService,
		tasksService,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/prj_1/tasks/task_1/start", bytes.NewBufferString(`{"runId":"run_1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer aitask_at_tok.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if got, want := body.Code, "PROJECT_ACCESS_DENIED"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}

	if err := agentsMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("agents expectations were not met: %v", err)
	}
	if err := tasksMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("tasks expectations were not met: %v", err)
	}
}
