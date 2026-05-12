package openviking

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClientReadClassifiesServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Read(context.Background(), "prj_1", "viking://x")
	if err == nil {
		t.Fatal("Read() error = nil, want error")
	}
	var ovErr *Error
	if !errors.As(err, &ovErr) {
		t.Fatalf("Read() error = %T, want *Error", err)
	}
	if got, want := ovErr.Kind, ErrorKindUnavailable; got != want {
		t.Fatalf("Error.Kind = %q, want %q", got, want)
	}
}

func TestClientWriteSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uri":"viking://aitask/projects/prj_1/memory/decisions/a.md","synced":true}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := client.Write(context.Background(), "prj_1", WriteInput{Target: "decisions", Title: "A", Content: "# A"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got, want := result.Synced, true; got != want {
		t.Fatalf("Synced = %v, want %v", got, want)
	}
}

func TestClientRegisterGitResourceUsesNativeResourceEndpoint(t *testing.T) {
	var gotPath, gotAuth, gotAPIKey string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-Api-Key")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{"uri":"viking://resources/aitask","path":"git@example.com:org/aitask.git"}}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask", APIKey: "secret"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := client.RegisterGitResource(context.Background(), GitResourceInput{
		RepositoryURL: "git@example.com:org/aitask.git",
		TargetURI:     "viking://resources/aitask",
		Reason:        "AITask source repository",
		WatchInterval: 5,
		Wait:          false,
		Branch:        "main",
		Commit:        "abc123",
	})
	if err != nil {
		t.Fatalf("RegisterGitResource() error = %v", err)
	}
	if gotPath != "/api/v1/resources" {
		t.Fatalf("path = %q, want /api/v1/resources", gotPath)
	}
	if gotAuth != "Bearer secret" || gotAPIKey != "secret" {
		t.Fatalf("auth headers = %q/%q, want bearer+x-api-key", gotAuth, gotAPIKey)
	}
	if gotBody["path"] != "git@example.com:org/aitask.git" || gotBody["to"] != "viking://resources/aitask" {
		t.Fatalf("unexpected body: %#v", gotBody)
	}
	if gotBody["watch_interval"] != float64(5) || gotBody["reason"] != "AITask source repository" {
		t.Fatalf("body missing watch/reason: %#v", gotBody)
	}
	if result.URI != "viking://resources/aitask" || result.RepositoryURL != "git@example.com:org/aitask.git" || !result.Synced {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestClientRegisterGitResourceTreatsWatchConflictAsSynced(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"status":"error","error":{"code":"CONFLICT","message":"Target URI 'viking://resources/aitask' is already being monitored by task c4cc8962-2579-4c44-a87f-660fefe52ff0. Please cancel the existing task first."}}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := client.RegisterGitResource(context.Background(), GitResourceInput{
		RepositoryURL: "git@example.com:org/aitask.git",
		TargetURI:     "viking://resources/aitask",
		WatchInterval: 5,
	})
	if err != nil {
		t.Fatalf("RegisterGitResource() error = %v", err)
	}
	if !result.Synced || result.TaskID != "c4cc8962-2579-4c44-a87f-660fefe52ff0" || result.URI != "viking://resources/aitask" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestClientResourceSyncStatusByTargetURI(t *testing.T) {
	var gotTaskQuery, gotAuth string
	var gotPaths []string
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotPaths = append(gotPaths, r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/tasks":
			gotTaskQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"status":"ok","result":[{"task_id":"task_1","task_type":"add_resource","status":"running","resource_id":"viking://resources/aitask","created_at":1,"updated_at":2}]}`))
		case "/api/v1/fs/ls":
			_, _ = w.Write([]byte(`{"status":"ok","result":[{"uri":"viking://resources/aitask/README.md","isDir":false}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask", APIKey: "secret"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := client.ResourceSyncStatus(context.Background(), ResourceTaskQuery{TargetURI: "viking://resources/aitask", Limit: 5})
	if err != nil {
		t.Fatalf("ResourceSyncStatus() error = %v", err)
	}
	if len(gotPaths) == 0 || gotPaths[0] != "/api/v1/tasks" {
		t.Fatalf("paths = %#v, first want /api/v1/tasks", gotPaths)
	}
	values, _ := url.ParseQuery(gotTaskQuery)
	if values.Get("resource_id") != "viking://resources/aitask" || values.Get("limit") != "5" {
		t.Fatalf("query = %q", gotTaskQuery)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q, want bearer", gotAuth)
	}
	if !result.Monitored || result.Current == nil || result.Current.TaskID != "task_1" || result.Current.Status != "running" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !result.Indexed || result.IndexedItems != 1 || calls != 2 {
		t.Fatalf("indexed/calls = %t/%d/%d, want true/1/2", result.Indexed, result.IndexedItems, calls)
	}
}

func TestClientResourceSyncStatusUsesIndexedResourceWhenTaskListEmpty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/tasks":
			_, _ = w.Write([]byte(`{"status":"ok","result":[]}`))
		case "/api/v1/fs/ls":
			_, _ = w.Write([]byte(`{"status":"ok","result":[{"uri":"viking://resources/aitask/go.mod","isDir":false},{"uri":"viking://resources/aitask/core","isDir":true}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := client.ResourceSyncStatus(context.Background(), ResourceTaskQuery{TargetURI: "viking://resources/aitask"})
	if err != nil {
		t.Fatalf("ResourceSyncStatus() error = %v", err)
	}
	if !result.Monitored || !result.Indexed || result.Status != "indexed" || result.IndexedItems != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestClientResourceSyncStatusByTaskID(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{"task_id":"task_1","status":"completed","resource_id":"viking://resources/aitask"}}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := client.ResourceSyncStatus(context.Background(), ResourceTaskQuery{TaskID: "task_1"})
	if err != nil {
		t.Fatalf("ResourceSyncStatus() error = %v", err)
	}
	if gotPath != "/api/v1/tasks/task_1" {
		t.Fatalf("path = %q, want /api/v1/tasks/task_1", gotPath)
	}
	if result.Current == nil || result.Current.Status != "completed" || result.Current.ResourceID != "viking://resources/aitask" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestClientWriteRejectsDisallowedTarget(t *testing.T) {
	client, err := New(Options{BaseURL: "http://127.0.0.1:8080", Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = client.Write(context.Background(), "prj_1", WriteInput{
		Target:  "task_status",
		Title:   "x",
		Content: "x",
	})
	if err == nil {
		t.Fatal("Write() error = nil, want disallowed target error")
	}
	var ovErr *Error
	if !errors.As(err, &ovErr) {
		t.Fatalf("Write() error = %T, want *Error", err)
	}
	if got, want := ovErr.Kind, ErrorKindBadRequest; got != want {
		t.Fatalf("Error.Kind = %q, want %q", got, want)
	}
}

func TestClientWriteClassifiesRedirectAsUnavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://example.com/sys/login?redirect=abc")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer ts.Close()

	noFollow := ts.Client()
	noFollow.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask", HTTPClient: noFollow})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Write(context.Background(), "prj_1", WriteInput{Target: "decision", Title: "x", Content: "x"})
	if err == nil {
		t.Fatal("Write() error = nil, want redirect classification error")
	}

	var ovErr *Error
	if !errors.As(err, &ovErr) {
		t.Fatalf("Write() error = %T, want *Error", err)
	}
	if got, want := ovErr.Kind, ErrorKindUnavailable; got != want {
		t.Fatalf("Error.Kind = %q, want %q", got, want)
	}
	if !strings.Contains(ovErr.Error(), "redirect") {
		t.Fatalf("Error() = %q, want redirect hint", ovErr.Error())
	}
}

func TestClientWriteClassifiesHTMLBodyAsUnavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>login required</body></html>"))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Write(context.Background(), "prj_1", WriteInput{Target: "decision", Title: "x", Content: "x"})
	if err == nil {
		t.Fatal("Write() error = nil, want html body classification error")
	}

	var ovErr *Error
	if !errors.As(err, &ovErr) {
		t.Fatalf("Write() error = %T, want *Error", err)
	}
	if got, want := ovErr.Kind, ErrorKindUnavailable; got != want {
		t.Fatalf("Error.Kind = %q, want %q", got, want)
	}
	if !strings.Contains(ovErr.Error(), "HTML") {
		t.Fatalf("Error() = %q, want HTML hint", ovErr.Error())
	}
}

func TestClientWriteUsesModernEndpointBeforeLegacy(t *testing.T) {
	var modernCalled, legacyCalled bool
	var capturedURI string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/content/write":
			modernCalled = true
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode modern write request failed: %v", err)
			}
			capturedURI = strings.TrimSpace(anyString(req["uri"]))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","result":{"uri":"viking://aitask/projects/prj_1/memory/decisions/a.md","content_updated":true}}`))
		case "/api/v1/namespaces/aitask/projects/prj_1/memory/write":
			legacyCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uri":"viking://aitask/projects/prj_1/memory/decisions/a.md","synced":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := client.Write(context.Background(), "prj_1", WriteInput{Target: "decisions", Title: "A", Content: "# A"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if !modernCalled {
		t.Fatal("modern endpoint was not called")
	}
	if legacyCalled {
		t.Fatal("legacy endpoint should not be called when modern succeeds")
	}
	if got, want := capturedURI, "viking://user/aitask/projects/prj_1/memories/decisions/A.md"; got != want {
		t.Fatalf("modern uri = %q, want %q", got, want)
	}
	if got, want := result.URI, "viking://aitask/projects/prj_1/memory/decisions/a.md"; got != want {
		t.Fatalf("result.URI = %q, want %q", got, want)
	}
}

func TestClientWriteFallsBackToLegacyWhenModernNotFound(t *testing.T) {
	var modernCalled, legacyCalled bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/content/write":
			modernCalled = true
			http.NotFound(w, r)
		case "/api/v1/namespaces/aitask/projects/prj_1/memory/write":
			legacyCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uri":"viking://aitask/projects/prj_1/memory/decisions/fallback.md","synced":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := client.Write(context.Background(), "prj_1", WriteInput{Target: "decisions", Title: "fallback", Content: "# fallback"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if !modernCalled || !legacyCalled {
		t.Fatalf("expected modern+legacy calls, got modern=%v legacy=%v", modernCalled, legacyCalled)
	}
	if got, want := result.URI, "viking://aitask/projects/prj_1/memory/decisions/fallback.md"; got != want {
		t.Fatalf("result.URI = %q, want %q", got, want)
	}
}

func TestClientWriteRetriesCreateBeforeLegacyOnModernReplaceNotFound(t *testing.T) {
	var modernCalls int
	var legacyCalled bool
	var modes []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/content/write":
			modernCalls++
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			modes = append(modes, strings.TrimSpace(anyString(req["mode"])))
			// Simulate "replace requires existing file", "create succeeds".
			if strings.EqualFold(strings.TrimSpace(anyString(req["mode"])), "replace") {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"status":"error","result":null,"error":{"code":"NOT_FOUND","message":"File not found"}}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","result":{"uri":"viking://user/aitask/projects/prj_1/memories/decisions/created.md","content_updated":true}}`))
		case "/api/v1/namespaces/aitask/projects/prj_1/memory/write":
			legacyCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uri":"viking://legacy","synced":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := client.Write(context.Background(), "prj_1", WriteInput{Target: "decisions", Title: "created", Content: "# created"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if legacyCalled {
		t.Fatal("legacy endpoint should not be called when modern create retry succeeds")
	}
	if modernCalls < 2 {
		t.Fatalf("modern calls = %d, want at least 2 (replace then create)", modernCalls)
	}
	if len(modes) < 2 || modes[0] != "replace" || modes[1] != "create" {
		t.Fatalf("modern modes = %v, want [replace create ...]", modes)
	}
	if got, want := result.URI, "viking://user/aitask/projects/prj_1/memories/decisions/created.md"; got != want {
		t.Fatalf("result.URI = %q, want %q", got, want)
	}
}

func TestClientWriteRetriesWhenResourceBusy(t *testing.T) {
	var createCalls int
	var legacyCalled bool
	var modes []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/content/write":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			mode := strings.TrimSpace(anyString(req["mode"]))
			modes = append(modes, mode)

			if mode == "replace" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"status":"error","result":null,"error":{"code":"NOT_FOUND","message":"File not found"}}`))
				return
			}

			createCalls++
			if createCalls == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"status":"error","result":null,"error":{"code":"INVALID_ARGUMENT","message":"resource is busy and cannot be written now: viking://user/aitask/projects/prj_1/memories/decisions/retry.md","details":{}}}`))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","result":{"uri":"viking://user/aitask/projects/prj_1/memories/decisions/retry.md","content_updated":true}}`))
		case "/api/v1/namespaces/aitask/projects/prj_1/memory/write":
			legacyCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uri":"viking://legacy","synced":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := client.Write(context.Background(), "prj_1", WriteInput{Target: "decisions", Title: "retry", Content: "# retry"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if legacyCalled {
		t.Fatal("legacy endpoint should not be called when resource busy retry succeeds")
	}
	if createCalls != 2 {
		t.Fatalf("create calls = %d, want 2 (busy then success)", createCalls)
	}
	if len(modes) < 3 || modes[0] != "replace" || modes[1] != "create" || modes[2] != "create" {
		t.Fatalf("modern modes = %v, want [replace create create ...]", modes)
	}
	if got, want := result.URI, "viking://user/aitask/projects/prj_1/memories/decisions/retry.md"; got != want {
		t.Fatalf("result.URI = %q, want %q", got, want)
	}
}

func TestClientDoSendsBothAuthHeaders(t *testing.T) {
	var auth, xAPIKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		xAPIKey = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{"memories":[],"resources":[],"skills":[]}}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask", APIKey: "secret-token"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = client.Search(context.Background(), "prj_1", "hello world", 0, false)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if got, want := auth, "Bearer secret-token"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
	if got, want := xAPIKey, "secret-token"; got != want {
		t.Fatalf("X-Api-Key = %q, want %q", got, want)
	}
}

func TestResolveWriteURICompatibility(t *testing.T) {
	got := resolveWriteURI("aitask", "prj_1", "handoff", "../A B")
	want := "viking://user/aitask/projects/prj_1/memories/handoffs/A-B.md"
	if got != want {
		t.Fatalf("resolveWriteURI() = %q, want %q", got, want)
	}
}

func TestClientListModernShapeNormalization(t *testing.T) {
	var requestedURI string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fs/ls" {
			http.NotFound(w, r)
			return
		}
		requestedURI = r.URL.Query().Get("uri")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"ok",
			"result":[
				{"uri":"viking://aitask/projects/prj_1/memory/decisions","isDir":true},
				{"uri":"viking://aitask/projects/prj_1/memory/decisions/abc.md","isDir":false},
				{"uri":"viking://aitask/projects/prj_1/memory/decisions/.overview.md","isDir":false}
			]
		}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	items, err := client.List(context.Background(), "prj_1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := requestedURI, "viking://user/aitask/projects/prj_1"; got != want {
		t.Fatalf("fs/ls uri query = %q, want %q", got, want)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if got, want := items[0].URI, "viking://aitask/projects/prj_1/memory/decisions/abc.md"; got != want {
		t.Fatalf("items[0].URI = %q, want %q", got, want)
	}
}

func TestClientReadModernShapeNormalization(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/content/read" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":"# hello"}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	entry, err := client.Read(context.Background(), "prj_1", "viking://aitask/projects/prj_1/memory/decisions/abc.md")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got, want := entry.ContentType, "text/markdown"; got != want {
		t.Fatalf("ContentType = %q, want %q", got, want)
	}
	if got, want := entry.Title, "abc"; got != want {
		t.Fatalf("Title = %q, want %q", got, want)
	}
}

func TestClientSearchModernShapeNormalization(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search/search" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"ok",
			"result":{
				"memories":[{"uri":"viking://aitask/projects/prj_1/memory/decisions/a.md","abstract":"decision abstract"}],
				"resources":[{"uri":"viking://aitask/projects/prj_1/resources/api/README.md","overview":"resource overview"}],
				"skills":[]
			}
		}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	items, err := client.Search(context.Background(), "prj_1", "decision", 0, false)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if got := items[0].Snippet; got == "" {
		t.Fatalf("memory snippet is empty")
	}
}

func TestClientSearchEmptyQueryReturnsEmpty(t *testing.T) {
	client, err := New(Options{BaseURL: "http://127.0.0.1:8080", Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	items, err := client.Search(context.Background(), "prj_1", "   ", 0, false)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}
}

func TestSearchLimitFromBudgetBoundaries(t *testing.T) {
	tests := []struct {
		budget int
		want   int
	}{
		{budget: 0, want: 20},
		{budget: 1, want: 5},
		{budget: 300, want: 5},
		{budget: 2000, want: 10},
		{budget: 200000, want: 50},
	}
	for _, tc := range tests {
		if got := searchLimitFromBudget(tc.budget); got != tc.want {
			t.Fatalf("searchLimitFromBudget(%d) = %d, want %d", tc.budget, got, tc.want)
		}
	}
}

func TestClientListFallsBackToLegacyOnModern404(t *testing.T) {
	var modernCalled, legacyCalled bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/fs/ls":
			modernCalled = true
			http.NotFound(w, r)
		case "/api/v1/namespaces/aitask/projects/prj_1/memory":
			legacyCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"title":"legacy","uri":"viking://aitask/projects/prj_1/memory/legacy.md","type":"markdown"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	items, err := client.List(context.Background(), "prj_1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !modernCalled || !legacyCalled {
		t.Fatalf("expected modern+legacy calls, got modern=%v legacy=%v", modernCalled, legacyCalled)
	}
	if len(items) != 1 || items[0].Title != "legacy" {
		t.Fatalf("unexpected legacy fallback result: %+v", items)
	}
}

func TestClientReadFallsBackToLegacyOnModern404(t *testing.T) {
	var modernCalled, legacyCalled bool
	uri := "viking://aitask/projects/prj_1/memory/legacy.md"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/content/read":
			modernCalled = true
			http.NotFound(w, r)
		case "/api/v1/namespaces/aitask/projects/prj_1/memory/read":
			legacyCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uri":"viking://aitask/projects/prj_1/memory/legacy.md","title":"legacy","contentType":"text/markdown","content":"legacy content"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	entry, err := client.Read(context.Background(), "prj_1", uri)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !modernCalled || !legacyCalled {
		t.Fatalf("expected modern+legacy calls, got modern=%v legacy=%v", modernCalled, legacyCalled)
	}
	if got, want := entry.Content, "legacy content"; got != want {
		t.Fatalf("entry.Content = %q, want %q", got, want)
	}
}

func TestClientSearchFallsBackToLegacyOnModern404(t *testing.T) {
	var modernCalled, legacyCalled bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/search/search":
			modernCalled = true
			http.NotFound(w, r)
		case "/api/v1/namespaces/aitask/projects/prj_1/memory/search":
			legacyCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"uri":"viking://aitask/projects/prj_1/memory/legacy.md","title":"legacy","snippet":"legacy snippet"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	items, err := client.Search(context.Background(), "prj_1", "legacy", 0, false)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !modernCalled || !legacyCalled {
		t.Fatalf("expected modern+legacy calls, got modern=%v legacy=%v", modernCalled, legacyCalled)
	}
	if len(items) != 1 || items[0].Title != "legacy" {
		t.Fatalf("unexpected legacy fallback items: %+v", items)
	}
}

func TestClientListPropagatesModernUnauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/fs/ls" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":"error","error":{"code":"UNAUTHENTICATED","message":"bad key"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = client.List(context.Background(), "prj_1")
	if err == nil {
		t.Fatal("List() error = nil, want unauthorized error")
	}
	var ovErr *Error
	if !errors.As(err, &ovErr) {
		t.Fatalf("List() error = %T, want *Error", err)
	}
	if got, want := ovErr.Kind, ErrorKindUnauthorized; got != want {
		t.Fatalf("Error.Kind = %q, want %q", got, want)
	}
}

func TestLastSegmentFallsBackOnInvalidURI(t *testing.T) {
	if got, want := lastSegment("not a uri/path.txt"), "path.txt"; got != want {
		t.Fatalf("lastSegment() = %q, want %q", got, want)
	}
}

func TestPathCleanAndJoin(t *testing.T) {
	if got, want := pathClean(`/a/../b/./c`), "/b/c"; got != want {
		t.Fatalf("pathClean() = %q, want %q", got, want)
	}
	if got, want := pathJoin("a", "/b/", "c"), "a/b/c"; got != want {
		t.Fatalf("pathJoin() = %q, want %q", got, want)
	}
}

func TestNormalizeTitlePath(t *testing.T) {
	if got, want := normalizeTitlePath("../x y/z"), "x-y/z.md"; got != want {
		t.Fatalf("normalizeTitlePath() = %q, want %q", got, want)
	}
}

func TestNormalizeModernReadRejectsUnexpectedShape(t *testing.T) {
	out := normalizeModernRead("viking://aitask/projects/prj_1/memory/a.md", map[string]any{
		"status": "ok",
		"result": map[string]any{"content": "x"},
	})
	if out != nil {
		t.Fatalf("normalizeModernRead() = %+v, want nil", out)
	}
}

func TestClientModernEndpointReceivesTargetURI(t *testing.T) {
	var targetURI string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search/search" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			TargetURI any `json:"target_uri"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		targetURI = anyString(req.TargetURI)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{"memories":[],"resources":[],"skills":[]}}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = client.Search(context.Background(), "prj_1", "test", 0, false)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if got, want := targetURI, "viking://user/aitask/projects/prj_1"; got != want {
		t.Fatalf("target_uri = %q, want %q", got, want)
	}
}

func TestClientListModernURIIsQueryEscaped(t *testing.T) {
	var rawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fs/ls" {
			http.NotFound(w, r)
			return
		}
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":[]}`))
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL, Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = client.List(context.Background(), "prj_1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	values, _ := url.ParseQuery(rawQuery)
	if got, want := values.Get("uri"), "viking://user/aitask/projects/prj_1"; got != want {
		t.Fatalf("uri query = %q, want %q", got, want)
	}
}

func TestClientNormalizesConsoleBaseURL(t *testing.T) {
	var modernCalled bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ov-api/api/v1/fs/ls" {
			modernCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","result":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client, err := New(Options{BaseURL: ts.URL + "/console", Namespace: "aitask"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = client.List(context.Background(), "prj_1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !modernCalled {
		t.Fatal("expected modern endpoint call on /ov-api/api/v1/fs/ls")
	}
}

func TestParseTenantFromAPIKey(t *testing.T) {
	account, user := parseTenantFromAPIKey("ZGVmYXVsdA.YWl0YXNr.signature")
	if got, want := account, "default"; got != want {
		t.Fatalf("account = %q, want %q", got, want)
	}
	if got, want := user, "aitask"; got != want {
		t.Fatalf("user = %q, want %q", got, want)
	}
}

func TestClientWriteRetriesWithTenantHeadersOnRootKeyError(t *testing.T) {
	var firstAttempt, secondAttempt bool
	var headerAccount, headerUser string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/content/write" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-OpenViking-Account") == "" || r.Header.Get("X-OpenViking-User") == "" {
			firstAttempt = true
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":"error","result":null,"error":{"code":"INVALID_ARGUMENT","message":"ROOT requests to tenant-scoped APIs must include X-OpenViking-Account and X-OpenViking-User headers. Use a user key for regular data access.","details":{}}}`))
			return
		}
		secondAttempt = true
		headerAccount = r.Header.Get("X-OpenViking-Account")
		headerUser = r.Header.Get("X-OpenViking-User")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{"uri":"viking://user/aitask/projects/prj_1/memories/decisions/a.md","content_updated":true}}`))
	}))
	defer ts.Close()

	// default.aitask.signature in base64url-ish layout to simulate OpenViking user key format.
	client, err := New(Options{
		BaseURL:   ts.URL,
		Namespace: "aitask",
		APIKey:    "ZGVmYXVsdA.YWl0YXNr.signature",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := client.Write(context.Background(), "prj_1", WriteInput{Target: "decisions", Title: "A", Content: "# A"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if !firstAttempt || !secondAttempt {
		t.Fatalf("expected first+second attempt, got first=%v second=%v", firstAttempt, secondAttempt)
	}
	if got, want := headerAccount, "default"; got != want {
		t.Fatalf("X-OpenViking-Account = %q, want %q", got, want)
	}
	if got, want := headerUser, "aitask"; got != want {
		t.Fatalf("X-OpenViking-User = %q, want %q", got, want)
	}
	if got, want := result.URI, "viking://user/aitask/projects/prj_1/memories/decisions/a.md"; got != want {
		t.Fatalf("result.URI = %q, want %q", got, want)
	}
}
