package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
)

func TestOpenVikingResourcesRegisterGitResource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	registrar := &gitResourceRegistrarStub{
		result: openviking.GitResourceResult{
			URI:           "viking://resources/aitask",
			RepositoryURL: "git@example.com:org/aitask.git",
			WatchInterval: 5,
			Branch:        "main",
			Commit:        "abc123",
			Synced:        true,
		},
	}
	router := gin.New()
	handler := NewOpenVikingResourcesHandler(registrar)
	router.POST("/api/projects/:projectId/openviking/resources/git", handler.RegisterGitResource)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/prj_1/openviking/resources/git", bytes.NewBufferString(`{
		"repositoryUrl":"git@example.com:org/aitask.git",
		"targetUri":"viking://resources/aitask",
		"watchInterval":5,
		"wait":false,
		"branch":"main",
		"commit":"abc123"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if registrar.projectID != "prj_1" {
		t.Fatalf("projectID = %q, want prj_1", registrar.projectID)
	}
	if registrar.input.RepositoryURL != "git@example.com:org/aitask.git" || registrar.input.TargetURI != "viking://resources/aitask" {
		t.Fatalf("unexpected input: %#v", registrar.input)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["uri"] != "viking://resources/aitask" || body["repositoryUrl"] != "git@example.com:org/aitask.git" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestOpenVikingResourcesRejectsMissingRepositoryURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	registrar := &gitResourceRegistrarStub{}
	router := gin.New()
	handler := NewOpenVikingResourcesHandler(registrar)
	router.POST("/api/projects/:projectId/openviking/resources/git", handler.RegisterGitResource)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/prj_1/openviking/resources/git", bytes.NewBufferString(`{"targetUri":"viking://resources/aitask"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if registrar.calls != 0 {
		t.Fatalf("registrar calls = %d, want 0", registrar.calls)
	}
}

func TestOpenVikingResourcesMapsBadRequestError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	registrar := &gitResourceRegistrarStub{
		err: &openviking.Error{Op: "register_git_resource", Kind: openviking.ErrorKindBadRequest, Err: errors.New("target uri cannot be empty")},
	}
	router := gin.New()
	handler := NewOpenVikingResourcesHandler(registrar)
	router.POST("/api/projects/:projectId/openviking/resources/git", handler.RegisterGitResource)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/prj_1/openviking/resources/git", bytes.NewBufferString(`{"repositoryUrl":"git@example.com:org/aitask.git","targetUri":"viking://resources/aitask"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestOpenVikingResourcesGitResourceStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	registrar := &gitResourceRegistrarStub{
		status: openviking.ResourceSyncStatus{
			TargetURI: "viking://resources/aitask",
			Monitored: true,
			Items: []openviking.ResourceTaskStatus{{
				TaskID:   "task_1",
				TaskType: "watch",
				Status:   "running",
			}},
		},
	}
	router := gin.New()
	handler := NewOpenVikingResourcesHandler(registrar)
	router.GET("/api/projects/:projectId/openviking/resources/git/status", handler.GitResourceStatus)

	req := httptest.NewRequest(http.MethodGet, "/api/projects/prj_1/openviking/resources/git/status?targetUri=viking://resources/aitask&limit=5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if registrar.statusQuery.TargetURI != "viking://resources/aitask" || registrar.statusQuery.Limit != 5 {
		t.Fatalf("unexpected status query: %#v", registrar.statusQuery)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["targetUri"] != "viking://resources/aitask" || body["monitored"] != true {
		t.Fatalf("unexpected body: %#v", body)
	}
}

type gitResourceRegistrarStub struct {
	projectID   string
	input       openviking.GitResourceInput
	result      openviking.GitResourceResult
	status      openviking.ResourceSyncStatus
	statusQuery openviking.ResourceTaskQuery
	err         error
	statusErr   error
	calls       int
}

func (s *gitResourceRegistrarStub) RegisterGitResource(_ context.Context, projectID string, input openviking.GitResourceInput) (openviking.GitResourceResult, error) {
	s.calls++
	s.projectID = projectID
	s.input = input
	if s.err != nil {
		return openviking.GitResourceResult{}, s.err
	}
	return s.result, nil
}

func (s *gitResourceRegistrarStub) ResourceSyncStatus(_ context.Context, projectID string, query openviking.ResourceTaskQuery) (openviking.ResourceSyncStatus, error) {
	s.projectID = projectID
	s.statusQuery = query
	if s.statusErr != nil {
		return openviking.ResourceSyncStatus{}, s.statusErr
	}
	return s.status, nil
}
