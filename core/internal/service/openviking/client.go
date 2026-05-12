package openviking

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ErrorKind string

const (
	ErrorKindUnavailable  ErrorKind = "unavailable"
	ErrorKindUnauthorized ErrorKind = "unauthorized"
	ErrorKindBadRequest   ErrorKind = "bad_request"
	ErrorKindUnexpected   ErrorKind = "unexpected"
)

type Error struct {
	Op         string
	Kind       ErrorKind
	StatusCode int
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("openviking %s failed (%s, status=%d): %v", e.Op, e.Kind, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("openviking %s failed (%s): %v", e.Op, e.Kind, e.Err)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type ListItem struct {
	Title string `json:"title"`
	URI   string `json:"uri"`
	Type  string `json:"type"`
}

type SearchHit struct {
	URI             string `json:"uri"`
	Title           string `json:"title"`
	Snippet         string `json:"snippet"`
	EstimatedTokens int    `json:"estimatedTokens,omitempty"`
}

type ReadEntry struct {
	URI         string `json:"uri"`
	Title       string `json:"title"`
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type WriteInput struct {
	Target         string `json:"target"`
	Title          string `json:"title"`
	Content        string `json:"content"`
	RelatedTaskID  string `json:"relatedTaskId,omitempty"`
	RelatedEventID string `json:"relatedEventId,omitempty"`
	AutoSync       bool   `json:"autoSync,omitempty"`
}

type WriteResult struct {
	URI    string `json:"uri"`
	Synced bool   `json:"synced"`
}

type GitResourceInput struct {
	RepositoryURL string `json:"repositoryUrl"`
	TargetURI     string `json:"targetUri"`
	Reason        string `json:"reason,omitempty"`
	WatchInterval int    `json:"watchInterval,omitempty"`
	Wait          bool   `json:"wait,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Commit        string `json:"commit,omitempty"`
}

type GitResourceResult struct {
	URI           string `json:"uri"`
	RepositoryURL string `json:"repositoryUrl"`
	WatchInterval int    `json:"watchInterval,omitempty"`
	TaskID        string `json:"taskId,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Commit        string `json:"commit,omitempty"`
	Synced        bool   `json:"synced"`
}

type ResourceTaskQuery struct {
	TaskID     string
	TargetURI  string
	TaskType   string
	Status     string
	Limit      int
	Repository string
}

type ResourceTaskStatus struct {
	TaskID     string         `json:"taskId"`
	TaskType   string         `json:"taskType,omitempty"`
	Status     string         `json:"status"`
	ResourceID string         `json:"resourceId,omitempty"`
	CreatedAt  float64        `json:"createdAt,omitempty"`
	UpdatedAt  float64        `json:"updatedAt,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	Error      map[string]any `json:"error,omitempty"`
}

type ResourceSyncStatus struct {
	TargetURI     string               `json:"targetUri,omitempty"`
	TaskID        string               `json:"taskId,omitempty"`
	WatchTaskID   string               `json:"watchTaskId,omitempty"`
	Items         []ResourceTaskStatus `json:"items"`
	Current       *ResourceTaskStatus  `json:"current,omitempty"`
	Monitored     bool                 `json:"monitored"`
	Indexed       bool                 `json:"indexed"`
	IndexedItems  int                  `json:"indexedItems,omitempty"`
	Status        string               `json:"status,omitempty"`
	Note          string               `json:"note,omitempty"`
	Repository    string               `json:"repository,omitempty"`
}

type doOptions struct {
	allowNotFound bool
	extraHeaders  map[string]string
}

type Client struct {
	baseURL    string
	namespace  string
	apiKey     string
	httpClient *http.Client
}

const (
	modernWriteBusyMaxAttempts = 5
	modernWriteBusyBaseDelay   = 200 * time.Millisecond
)

var allowedWriteTargets = map[string]struct{}{
	"brief":     {},
	"memory":    {},
	"resource":  {},
	"resources": {},
	"skill":     {},
	"skills":    {},
	"tasks":     {},
	"sessions":  {},
	"decision":  {},
	"decisions": {},
	"summary":   {},
	"handoff":   {},
	"handoffs":  {},
	"note":      {},
	"report":    {},
}

type Options struct {
	BaseURL    string
	Namespace  string
	APIKey     string
	HTTPClient *http.Client
}

func New(opts Options) (*Client, error) {
	baseURL, err := NormalizeServerURL(opts.BaseURL)
	if err != nil {
		return nil, &Error{Op: "new_client", Kind: ErrorKindBadRequest, Err: err}
	}
	namespace := strings.TrimSpace(opts.Namespace)
	if baseURL == "" {
		return nil, errors.New("openviking base url cannot be empty")
	}
	if namespace == "" {
		return nil, errors.New("openviking namespace cannot be empty")
	}
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("invalid openviking base url: %w", err)
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &Client{baseURL: baseURL, namespace: namespace, apiKey: strings.TrimSpace(opts.APIKey), httpClient: client}, nil
}

func (c *Client) List(ctx context.Context, projectID string) ([]ListItem, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, &Error{Op: "list", Kind: ErrorKindBadRequest, Err: errors.New("project id cannot be empty")}
	}

	// Prefer modern OpenViking API, fall back to legacy namespace/project API.
	ovRoot := c.projectRootURI(projectID)
	modernEndpoint := fmt.Sprintf("%s/api/v1/fs/ls?uri=%s&recursive=true&simple=false&output=original", c.baseURL, url.QueryEscape(ovRoot))
	var modernResponse map[string]any
	if err := c.do(ctx, http.MethodGet, modernEndpoint, nil, &modernResponse, doOptions{allowNotFound: true}); err == nil {
		items := normalizeModernList(modernResponse)
		if items != nil {
			return items, nil
		}
	} else if shouldRetryWithTenantHeaders(err) {
		modernResponse = map[string]any{}
		if retryErr := c.do(ctx, http.MethodGet, modernEndpoint, nil, &modernResponse, doOptions{
			allowNotFound: true,
			extraHeaders:  c.tenantHeaders(ctx),
		}); retryErr == nil {
			items := normalizeModernList(modernResponse)
			if items != nil {
				return items, nil
			}
		} else if !isNotFoundError(retryErr) {
			return nil, retryErr
		}
	} else if !isNotFoundError(err) {
		return nil, err
	}

	legacyEndpoint := fmt.Sprintf("%s/api/v1/namespaces/%s/projects/%s/memory", c.baseURL, url.PathEscape(c.namespace), url.PathEscape(projectID))
	var legacyResponse struct {
		Items []ListItem `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, legacyEndpoint, nil, &legacyResponse); err != nil {
		return nil, err
	}
	if legacyResponse.Items == nil {
		return []ListItem{}, nil
	}
	return legacyResponse.Items, nil
}

func (c *Client) Search(ctx context.Context, projectID string, query string, budget int, refsOnly bool) ([]SearchHit, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, &Error{Op: "search", Kind: ErrorKindBadRequest, Err: errors.New("project id cannot be empty")}
	}

	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return []SearchHit{}, nil
	}

	// Prefer modern OpenViking API, fall back to legacy namespace/project API.
	modernEndpoint := fmt.Sprintf("%s/api/v1/search/search", c.baseURL)
	modernRequest := map[string]any{
		"query":      trimmedQuery,
		"target_uri": c.projectRootURI(projectID),
		"limit":      searchLimitFromBudget(budget),
	}
	var modernResponse map[string]any
	if err := c.do(ctx, http.MethodPost, modernEndpoint, modernRequest, &modernResponse, doOptions{allowNotFound: true}); err == nil {
		items := normalizeModernSearch(modernResponse, refsOnly)
		if items != nil {
			return items, nil
		}
	} else if shouldRetryWithTenantHeaders(err) {
		modernResponse = map[string]any{}
		if retryErr := c.do(ctx, http.MethodPost, modernEndpoint, modernRequest, &modernResponse, doOptions{
			allowNotFound: true,
			extraHeaders:  c.tenantHeaders(ctx),
		}); retryErr == nil {
			items := normalizeModernSearch(modernResponse, refsOnly)
			if items != nil {
				return items, nil
			}
		} else if !isNotFoundError(retryErr) {
			return nil, retryErr
		}
	} else if !isNotFoundError(err) {
		return nil, err
	}

	urlValues := url.Values{}
	urlValues.Set("q", trimmedQuery)
	if budget > 0 {
		urlValues.Set("budget", strconv.Itoa(budget))
	}
	if refsOnly {
		urlValues.Set("refsOnly", "true")
	}
	legacyEndpoint := fmt.Sprintf("%s/api/v1/namespaces/%s/projects/%s/memory/search", c.baseURL, url.PathEscape(c.namespace), url.PathEscape(projectID))
	if encoded := urlValues.Encode(); encoded != "" {
		legacyEndpoint += "?" + encoded
	}
	var legacyResponse struct {
		Items []SearchHit `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, legacyEndpoint, nil, &legacyResponse); err != nil {
		return nil, err
	}
	if legacyResponse.Items == nil {
		return []SearchHit{}, nil
	}
	return legacyResponse.Items, nil
}

func (c *Client) Read(ctx context.Context, projectID string, uri string) (ReadEntry, error) {
	if strings.TrimSpace(projectID) == "" {
		return ReadEntry{}, &Error{Op: "read", Kind: ErrorKindBadRequest, Err: errors.New("project id cannot be empty")}
	}
	if strings.TrimSpace(uri) == "" {
		return ReadEntry{}, &Error{Op: "read", Kind: ErrorKindBadRequest, Err: errors.New("uri cannot be empty")}
	}

	// Prefer modern OpenViking API, fall back to legacy namespace/project API.
	modernEndpoint := fmt.Sprintf("%s/api/v1/content/read?uri=%s", c.baseURL, url.QueryEscape(strings.TrimSpace(uri)))
	var modernResponse map[string]any
	if err := c.do(ctx, http.MethodGet, modernEndpoint, nil, &modernResponse, doOptions{allowNotFound: true}); err == nil {
		entry := normalizeModernRead(uri, modernResponse)
		if entry != nil {
			return *entry, nil
		}
	} else if shouldRetryWithTenantHeaders(err) {
		modernResponse = map[string]any{}
		if retryErr := c.do(ctx, http.MethodGet, modernEndpoint, nil, &modernResponse, doOptions{
			allowNotFound: true,
			extraHeaders:  c.tenantHeaders(ctx),
		}); retryErr == nil {
			entry := normalizeModernRead(uri, modernResponse)
			if entry != nil {
				return *entry, nil
			}
		} else if !isNotFoundError(retryErr) {
			return ReadEntry{}, retryErr
		}
	} else if !isNotFoundError(err) {
		return ReadEntry{}, err
	}

	legacyEndpoint := fmt.Sprintf("%s/api/v1/namespaces/%s/projects/%s/memory/read?uri=%s", c.baseURL, url.PathEscape(c.namespace), url.PathEscape(projectID), url.QueryEscape(uri))
	var legacyResponse ReadEntry
	if err := c.do(ctx, http.MethodGet, legacyEndpoint, nil, &legacyResponse); err != nil {
		return ReadEntry{}, err
	}
	return legacyResponse, nil
}

func (c *Client) Write(ctx context.Context, projectID string, input WriteInput) (WriteResult, error) {
	if strings.TrimSpace(projectID) == "" {
		return WriteResult{}, &Error{Op: "write", Kind: ErrorKindBadRequest, Err: errors.New("project id cannot be empty")}
	}
	input.Target = strings.TrimSpace(input.Target)
	input.Title = strings.TrimSpace(input.Title)
	if input.Target == "" || input.Title == "" || strings.TrimSpace(input.Content) == "" {
		return WriteResult{}, &Error{Op: "write", Kind: ErrorKindBadRequest, Err: errors.New("target/title/content cannot be empty")}
	}
	if !isAllowedWriteTarget(input.Target) {
		return WriteResult{}, &Error{Op: "write", Kind: ErrorKindBadRequest, Err: errors.New("target is not allowed")}
	}

	resolvedURI := resolveWriteURI(c.namespace, projectID, input.Target, input.Title)

	// Prefer modern OpenViking API, fall back to legacy namespace/project API.
	modernEndpoint := fmt.Sprintf("%s/api/v1/content/write", c.baseURL)
	modernRequest := map[string]any{
		"uri":     resolvedURI,
		"content": strings.TrimSpace(input.Content) + "\n",
		"wait":    !input.AutoSync,
	}
	if result, err := c.tryModernWrite(ctx, modernEndpoint, modernRequest, resolvedURI, "replace"); err == nil && result != nil {
		return *result, nil
	} else if err != nil {
		// OpenViking "replace" requires file to exist. For first-time writes on a
		// brand-new project tree, retry with "create" before considering legacy.
		if isNotFoundError(err) {
			if createResult, createErr := c.tryModernWrite(ctx, modernEndpoint, modernRequest, resolvedURI, "create"); createErr == nil && createResult != nil {
				return *createResult, nil
			} else if createErr != nil && !isNotFoundError(createErr) {
				return WriteResult{}, createErr
			}
		} else {
			return WriteResult{}, err
		}
	}

	legacyEndpoint := fmt.Sprintf("%s/api/v1/namespaces/%s/projects/%s/memory/write", c.baseURL, url.PathEscape(c.namespace), url.PathEscape(projectID))
	var legacyResponse WriteResult
	if err := c.do(ctx, http.MethodPost, legacyEndpoint, input, &legacyResponse); err != nil {
		return WriteResult{}, err
	}
	if legacyResponse.URI == "" {
		legacyResponse.URI = resolvedURI
	}
	return legacyResponse, nil
}

func (c *Client) RegisterGitResource(ctx context.Context, input GitResourceInput) (GitResourceResult, error) {
	repoURL := strings.TrimSpace(input.RepositoryURL)
	targetURI := strings.TrimSpace(input.TargetURI)
	if repoURL == "" {
		return GitResourceResult{}, &Error{Op: "register_git_resource", Kind: ErrorKindBadRequest, Err: errors.New("repository url cannot be empty")}
	}
	if targetURI == "" {
		return GitResourceResult{}, &Error{Op: "register_git_resource", Kind: ErrorKindBadRequest, Err: errors.New("target uri cannot be empty")}
	}
	watchInterval := input.WatchInterval
	if watchInterval < 0 {
		return GitResourceResult{}, &Error{Op: "register_git_resource", Kind: ErrorKindBadRequest, Err: errors.New("watch interval cannot be negative")}
	}

	endpoint := fmt.Sprintf("%s/api/v1/resources", c.baseURL)
	request := map[string]any{
		"path": repoURL,
		"to":   targetURI,
		"wait": input.Wait,
	}
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		request["reason"] = reason
	}
	if watchInterval > 0 {
		request["watch_interval"] = watchInterval
	}

	var response map[string]any
	if err := c.do(ctx, http.MethodPost, endpoint, request, &response, doOptions{allowNotFound: true}); err != nil {
		if shouldRetryWithTenantHeaders(err) {
			response = map[string]any{}
			if retryErr := c.do(ctx, http.MethodPost, endpoint, request, &response, doOptions{
				allowNotFound: true,
				extraHeaders:  c.tenantHeaders(ctx),
			}); retryErr != nil {
				if conflictResult, ok := gitResourceConflictResult(retryErr, targetURI, repoURL, watchInterval); ok {
					return conflictResult, nil
				}
				return GitResourceResult{}, retryErr
			}
		} else {
			if conflictResult, ok := gitResourceConflictResult(err, targetURI, repoURL, watchInterval); ok {
				return conflictResult, nil
			}
			return GitResourceResult{}, err
		}
	}

	result := normalizeGitResourceResult(response)
	if result.URI == "" {
		result.URI = targetURI
	}
	result.RepositoryURL = repoURL
	result.WatchInterval = watchInterval
	if result.TaskID == "" {
		result.TaskID = firstNonEmptyAnyString(response, "task_id", "taskId")
	}
	result.Branch = strings.TrimSpace(input.Branch)
	result.Commit = strings.TrimSpace(input.Commit)
	result.Synced = true
	return result, nil
}

func (c *Client) ResourceSyncStatus(ctx context.Context, query ResourceTaskQuery) (ResourceSyncStatus, error) {
	taskID := strings.TrimSpace(query.TaskID)
	targetURI := strings.TrimSpace(query.TargetURI)
	if taskID == "" && targetURI == "" {
		return ResourceSyncStatus{}, &Error{Op: "resource_sync_status", Kind: ErrorKindBadRequest, Err: errors.New("task id or target uri is required")}
	}
	repository := strings.TrimSpace(query.Repository)
	if taskID != "" {
		endpoint := fmt.Sprintf("%s/api/v1/tasks/%s", c.baseURL, url.PathEscape(taskID))
		var response map[string]any
		if err := c.do(ctx, http.MethodGet, endpoint, nil, &response, doOptions{allowNotFound: true}); err != nil {
			if shouldRetryWithTenantHeaders(err) {
				response = map[string]any{}
				if retryErr := c.do(ctx, http.MethodGet, endpoint, nil, &response, doOptions{
					allowNotFound: true,
					extraHeaders:  c.tenantHeaders(ctx),
				}); retryErr != nil {
					return ResourceSyncStatus{}, retryErr
				}
			} else {
				return ResourceSyncStatus{}, err
			}
		}
		item := normalizeResourceTaskStatus(response)
		if item.TaskID == "" {
			item.TaskID = taskID
		}
		return ResourceSyncStatus{
			TargetURI:  firstNonEmpty(targetURI, item.ResourceID),
			TaskID:     taskID,
			Items:      []ResourceTaskStatus{item},
			Current:    &item,
			Monitored:  true,
			Indexed:    strings.EqualFold(item.Status, "completed"),
			Status:     item.Status,
			Repository: repository,
		}, nil
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	values := url.Values{}
	values.Set("resource_id", targetURI)
	values.Set("limit", strconv.Itoa(limit))
	if taskType := strings.TrimSpace(query.TaskType); taskType != "" {
		values.Set("task_type", taskType)
	}
	if status := strings.TrimSpace(query.Status); status != "" {
		values.Set("status", status)
	}
	endpoint := fmt.Sprintf("%s/api/v1/tasks?%s", c.baseURL, values.Encode())
	var response map[string]any
	if err := c.do(ctx, http.MethodGet, endpoint, nil, &response, doOptions{allowNotFound: true}); err != nil {
		if shouldRetryWithTenantHeaders(err) {
			response = map[string]any{}
			if retryErr := c.do(ctx, http.MethodGet, endpoint, nil, &response, doOptions{
				allowNotFound: true,
				extraHeaders:  c.tenantHeaders(ctx),
			}); retryErr != nil {
				return ResourceSyncStatus{}, retryErr
			}
		} else {
			return ResourceSyncStatus{}, err
		}
	}
	items := normalizeResourceTaskList(response)
	var current *ResourceTaskStatus
	if len(items) > 0 {
		current = &items[0]
	}
	indexed, indexedItems, indexedErr := c.resourceIndexed(ctx, targetURI)
	status := "unknown"
	note := ""
	if current != nil && strings.TrimSpace(current.Status) != "" {
		status = strings.TrimSpace(current.Status)
	} else if indexed {
		status = "indexed"
		note = "Resource content is visible in OpenViking. OpenViking does not currently expose watch-task status through /api/v1/tasks; watch task ids are only returned during registration conflicts."
	} else if indexedErr != nil {
		status = "unknown"
		note = indexedErr.Error()
	}
	return ResourceSyncStatus{
		TargetURI:    targetURI,
		Items:        items,
		Current:      current,
		Monitored:    len(items) > 0 || indexed,
		Indexed:      indexed,
		IndexedItems: indexedItems,
		Status:       status,
		Note:         note,
		Repository:   repository,
	}, nil
}

func (c *Client) resourceIndexed(ctx context.Context, targetURI string) (bool, int, error) {
	targetURI = strings.TrimSpace(targetURI)
	if targetURI == "" {
		return false, 0, nil
	}
	endpoint := fmt.Sprintf("%s/api/v1/fs/ls?uri=%s&recursive=true&simple=false&output=original", c.baseURL, url.QueryEscape(targetURI))
	var response map[string]any
	if err := c.do(ctx, http.MethodGet, endpoint, nil, &response, doOptions{allowNotFound: true}); err != nil {
		if shouldRetryWithTenantHeaders(err) {
			response = map[string]any{}
			if retryErr := c.do(ctx, http.MethodGet, endpoint, nil, &response, doOptions{
				allowNotFound: true,
				extraHeaders:  c.tenantHeaders(ctx),
			}); retryErr != nil {
				if isNotFoundError(retryErr) {
					return false, 0, nil
				}
				return false, 0, retryErr
			}
		} else {
			if isNotFoundError(err) {
				return false, 0, nil
			}
			return false, 0, err
		}
	}
	items := normalizeModernList(response)
	return len(items) > 0, len(items), nil
}

func (c *Client) tryModernWrite(ctx context.Context, endpoint string, requestBase map[string]any, fallbackURI string, mode string) (*WriteResult, error) {
	payload := make(map[string]any, len(requestBase)+1)
	for k, v := range requestBase {
		payload[k] = v
	}
	payload["mode"] = strings.TrimSpace(mode)

	var modernResponse map[string]any
	if err := c.doModernWrite(ctx, endpoint, payload, &modernResponse, doOptions{allowNotFound: true}); err == nil {
		return normalizeModernWrite(fallbackURI, modernResponse), nil
	} else if shouldRetryWithTenantHeaders(err) {
		modernResponse = map[string]any{}
		retryErr := c.doModernWrite(ctx, endpoint, payload, &modernResponse, doOptions{
			allowNotFound: true,
			extraHeaders:  c.tenantHeaders(ctx),
		})
		if retryErr != nil {
			return nil, retryErr
		}
		return normalizeModernWrite(fallbackURI, modernResponse), nil
	} else {
		return nil, err
	}
}

func (c *Client) doModernWrite(ctx context.Context, endpoint string, payload any, into any, opts doOptions) error {
	var lastErr error
	for attempt := 0; attempt < modernWriteBusyMaxAttempts; attempt++ {
		lastErr = c.do(ctx, http.MethodPost, endpoint, payload, into, opts)
		if lastErr == nil {
			return nil
		}
		if !isResourceBusyWrite(lastErr) || attempt == modernWriteBusyMaxAttempts-1 {
			return lastErr
		}
		delay := modernWriteBusyBaseDelay * time.Duration(attempt+1)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return classifyTransportError(http.MethodPost+" "+endpoint, ctx.Err())
		case <-timer.C:
		}
	}
	return lastErr
}

func isAllowedWriteTarget(target string) bool {
	_, ok := allowedWriteTargets[strings.TrimSpace(strings.ToLower(target))]
	return ok
}

func (c *Client) do(ctx context.Context, method string, endpoint string, payload any, into any, opts ...doOptions) error {
	var body io.Reader
	var option doOptions
	if len(opts) > 0 {
		option = opts[0]
	}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return &Error{Op: method + " " + endpoint, Kind: ErrorKindUnexpected, Err: err}
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return &Error{Op: method + " " + endpoint, Kind: ErrorKindUnexpected, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(c.apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.apiKey))
		req.Header.Set("X-Api-Key", strings.TrimSpace(c.apiKey))
	}
	for k, v := range option.extraHeaders {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return classifyTransportError(method+" "+endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := strings.TrimSpace(resp.Header.Get("Location"))
		hint := fmt.Sprintf("unexpected redirect (status=%d)", resp.StatusCode)
		if location != "" {
			hint = fmt.Sprintf("%s to %s", hint, location)
			if looksLikeLoginRedirect(location) {
				hint += " (possible auth/login gateway in front of OpenViking API)"
			}
		}
		return &Error{
			Op:         method + " " + endpoint,
			Kind:       ErrorKindUnavailable,
			StatusCode: resp.StatusCode,
			Err:        errors.New(hint),
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		kind := classifyStatus(resp.StatusCode)
		text := strings.TrimSpace(string(message))
		if text == "" {
			text = http.StatusText(resp.StatusCode)
		}
		if option.allowNotFound && resp.StatusCode == http.StatusNotFound {
			return &Error{Op: method + " " + endpoint, Kind: kind, StatusCode: resp.StatusCode, Err: errors.New(text)}
		}
		return &Error{Op: method + " " + endpoint, Kind: kind, StatusCode: resp.StatusCode, Err: errors.New(text)}
	}

	if into == nil {
		return nil
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &Error{Op: method + " " + endpoint, Kind: ErrorKindUnexpected, StatusCode: resp.StatusCode, Err: err}
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return &Error{
			Op:         method + " " + endpoint,
			Kind:       ErrorKindUnexpected,
			StatusCode: resp.StatusCode,
			Err:        errors.New("empty response body"),
		}
	}
	if err := json.Unmarshal(raw, into); err != nil {
		kind := ErrorKindUnexpected
		trimmed := strings.TrimSpace(string(raw))
		if strings.HasPrefix(trimmed, "<") || looksLikeHTMLContentType(resp.Header.Get("Content-Type")) {
			kind = ErrorKindUnavailable
		}
		hint := fmt.Sprintf("response is not valid JSON (content-type=%q)", strings.TrimSpace(resp.Header.Get("Content-Type")))
		if strings.HasPrefix(trimmed, "<") {
			hint += "; response looks like HTML (possible auth/login gateway or wrong endpoint)"
		}
		return &Error{
			Op:         method + " " + endpoint,
			Kind:       kind,
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("%s: %w", hint, err),
		}
	}
	return nil
}

func (c *Client) projectRootURI(projectID string) string {
	return fmt.Sprintf("viking://user/%s/projects/%s", strings.TrimSpace(c.namespace), strings.TrimSpace(projectID))
}

func resolveWriteURI(namespace string, projectID string, target string, title string) string {
	base := "memories/notes"
	switch strings.TrimSpace(strings.ToLower(target)) {
	case "brief":
		base = "memories/brief"
	case "memory":
		base = "memories/notes"
	case "resource", "resources":
		base = "memories/resources"
	case "skill", "skills":
		base = "memories/skills"
	case "tasks":
		base = "memories/tasks"
	case "sessions":
		base = "memories/sessions"
	case "decision", "decisions":
		base = "memories/decisions"
	case "summary":
		base = "memories/summaries"
	case "handoff", "handoffs":
		base = "memories/handoffs"
	case "note":
		base = "memories/notes"
	case "report":
		base = "memories/reports"
	}
	relTitle := normalizeTitlePath(title)
	if relTitle == "" {
		relTitle = "untitled.md"
	}
	return fmt.Sprintf("viking://user/%s/projects/%s/%s/%s", strings.TrimSpace(namespace), strings.TrimSpace(projectID), strings.TrimPrefix(base, "/"), strings.TrimPrefix(relTitle, "/"))
}

func shouldRetryWithTenantHeaders(err error) bool {
	var ovErr *Error
	if !errors.As(err, &ovErr) {
		return false
	}
	if ovErr.StatusCode != http.StatusBadRequest {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(ovErr.Err.Error()))
	return strings.Contains(msg, "root requests to tenant-scoped apis must include") ||
		(strings.Contains(msg, "x-openviking-account") && strings.Contains(msg, "x-openviking-user"))
}

func isResourceBusyWrite(err error) bool {
	var ovErr *Error
	if !errors.As(err, &ovErr) {
		return false
	}
	if ovErr.StatusCode != http.StatusBadRequest {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(ovErr.Err.Error()))
	return strings.Contains(msg, "resource is busy") && strings.Contains(msg, "cannot be written now")
}

func (c *Client) tenantHeaders(ctx context.Context) map[string]string {
	account, user := parseTenantFromAPIKey(c.apiKey)
	if account == "" {
		if detected := c.detectTenantAccount(ctx); detected != "" {
			account = detected
		} else {
			account = "default"
		}
	}
	if user == "" {
		user = strings.TrimSpace(c.namespace)
	}
	if account == "" || user == "" {
		return nil
	}
	return map[string]string{
		"X-OpenViking-Account": account,
		"X-OpenViking-User":    user,
	}
}

func (c *Client) detectTenantAccount(ctx context.Context) string {
	endpoint := fmt.Sprintf("%s/api/v1/system/status", c.baseURL)
	var response map[string]any
	if err := c.do(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return ""
	}
	resultMap, ok := anyMap(response["result"])
	if !ok {
		return ""
	}
	return strings.TrimSpace(anyString(resultMap["user"]))
}

func parseTenantFromAPIKey(apiKey string) (account string, user string) {
	parts := strings.Split(strings.TrimSpace(apiKey), ".")
	if len(parts) < 2 {
		return "", ""
	}
	decode := func(seg string) string {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return ""
		}
		if raw, err := base64.RawStdEncoding.DecodeString(seg); err == nil {
			return strings.TrimSpace(string(raw))
		}
		if raw, err := base64.StdEncoding.DecodeString(seg); err == nil {
			return strings.TrimSpace(string(raw))
		}
		return ""
	}
	return decode(parts[0]), decode(parts[1])
}

func normalizeTitlePath(title string) string {
	cleaned := strings.ReplaceAll(strings.TrimSpace(title), "\\", "/")
	cleaned = strings.TrimPrefix(pathClean("/"+cleaned), "/")
	if cleaned == "." || cleaned == "" {
		return ""
	}
	parts := strings.Split(cleaned, "/")
	sanitized := make([]string, 0, len(parts))
	for i, part := range parts {
		part = sanitizePathSegment(part)
		if part == "" {
			part = "untitled"
		}
		if i == len(parts)-1 && !strings.HasSuffix(strings.ToLower(part), ".md") {
			part += ".md"
		}
		sanitized = append(sanitized, part)
	}
	return pathJoin(sanitized...)
}

func sanitizePathSegment(input string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(input) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func normalizeModernList(body map[string]any) []ListItem {
	entries, ok := firstMapSlice(body, "result", "items")
	if !ok {
		return nil
	}
	items := make([]ListItem, 0, len(entries))
	for _, entry := range entries {
		uri := strings.TrimSpace(anyString(entry["uri"]))
		if uri == "" {
			continue
		}
		isDir := anyBool(entry["isDir"])
		if isDir {
			continue
		}
		if strings.HasPrefix(strings.ToLower(lastSegment(uri)), ".") {
			continue
		}
		title := stripMarkdownExt(lastSegment(uri))
		items = append(items, ListItem{
			Title: title,
			URI:   uri,
			Type:  inferItemType(uri),
		})
	}
	return items
}

func normalizeModernSearch(body map[string]any, refsOnly bool) []SearchHit {
	resultMap, ok := anyMap(body["result"])
	if !ok {
		return nil
	}
	// OpenViking search/search returns {status:"ok", result:{memories/resources/skills/...}}
	buckets := []string{"memories", "resources", "skills"}
	items := make([]SearchHit, 0)
	for _, bucket := range buckets {
		rawList, ok := anySlice(resultMap[bucket])
		if !ok {
			continue
		}
		for _, raw := range rawList {
			entry, ok := anyMap(raw)
			if !ok {
				continue
			}
			uri := strings.TrimSpace(anyString(entry["uri"]))
			if uri == "" {
				continue
			}
			abstract := strings.TrimSpace(anyString(entry["abstract"]))
			overview := strings.TrimSpace(anyString(entry["overview"]))
			title := stripMarkdownExt(lastSegment(uri))
			snippet := ""
			if !refsOnly {
				if abstract != "" {
					snippet = abstract
				} else if overview != "" {
					snippet = overview
				}
			}
			items = append(items, SearchHit{
				URI:     uri,
				Title:   title,
				Snippet: snippet,
			})
		}
	}
	return items
}

func normalizeGitResourceResult(body map[string]any) GitResourceResult {
	result := GitResourceResult{}
	candidates := []map[string]any{body}
	if resultMap, ok := anyMap(body["result"]); ok {
		candidates = append([]map[string]any{resultMap}, candidates...)
	}
	if dataMap, ok := anyMap(body["data"]); ok {
		candidates = append([]map[string]any{dataMap}, candidates...)
	}
	for _, item := range candidates {
		if result.URI == "" {
			result.URI = firstNonEmptyAnyString(item, "uri", "resource_uri", "resourceUri", "to", "target_uri", "targetUri")
		}
		if result.RepositoryURL == "" {
			result.RepositoryURL = firstNonEmptyAnyString(item, "path", "repository_url", "repositoryUrl", "url")
		}
		if result.TaskID == "" {
			result.TaskID = firstNonEmptyAnyString(item, "task_id", "taskId", "id")
		}
	}
	return result
}

func gitResourceConflictResult(err error, targetURI, repoURL string, watchInterval int) (GitResourceResult, bool) {
	var ovErr *Error
	if !errors.As(err, &ovErr) || ovErr.StatusCode != http.StatusConflict {
		return GitResourceResult{}, false
	}
	taskID := watchTaskIDFromText(ovErr.Error())
	if taskID == "" {
		return GitResourceResult{}, false
	}
	return GitResourceResult{
		URI:           targetURI,
		RepositoryURL: repoURL,
		WatchInterval: watchInterval,
		TaskID:        taskID,
		Synced:        true,
	}, true
}

func watchTaskIDFromText(value string) string {
	const marker = "task "
	idx := strings.Index(value, marker)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(value[idx+len(marker):])
	if rest == "" {
		return ""
	}
	end := 0
	for end < len(rest) {
		ch := rest[end]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			end++
			continue
		}
		break
	}
	return strings.TrimSpace(rest[:end])
}

func normalizeResourceTaskStatus(body map[string]any) ResourceTaskStatus {
	item := body
	if resultMap, ok := anyMap(body["result"]); ok {
		item = resultMap
	}
	return resourceTaskStatusFromMap(item)
}

func normalizeResourceTaskList(body map[string]any) []ResourceTaskStatus {
	var raw []map[string]any
	if resultMap, ok := anyMap(body["result"]); ok {
		if items, ok := firstMapSlice(resultMap, "items", "tasks", "data"); ok {
			raw = items
		} else if list, ok := mapSlice(resultMap["result"]); ok {
			raw = list
		}
	}
	if len(raw) == 0 {
		if items, ok := firstMapSlice(body, "items", "tasks", "data", "result"); ok {
			raw = items
		}
	}
	out := make([]ResourceTaskStatus, 0, len(raw))
	for _, item := range raw {
		normalized := resourceTaskStatusFromMap(item)
		if normalized.TaskID == "" && normalized.Status == "" {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func resourceTaskStatusFromMap(item map[string]any) ResourceTaskStatus {
	resultMap, _ := anyMap(item["result"])
	errorMap, _ := anyMap(item["error"])
	return ResourceTaskStatus{
		TaskID:     firstNonEmptyAnyString(item, "task_id", "taskId", "id"),
		TaskType:   firstNonEmptyAnyString(item, "task_type", "taskType", "type"),
		Status:     firstNonEmptyAnyString(item, "status", "state"),
		ResourceID: firstNonEmptyAnyString(item, "resource_id", "resourceId", "target_uri", "targetUri", "uri"),
		CreatedAt:  anyFloat(item["created_at"], item["createdAt"], item["created"]),
		UpdatedAt:  anyFloat(item["updated_at"], item["updatedAt"], item["updated"]),
		Result:     resultMap,
		Error:      errorMap,
	}
}

func normalizeModernRead(requestURI string, body map[string]any) *ReadEntry {
	result, ok := body["result"]
	if !ok {
		return nil
	}
	var content string
	switch typed := result.(type) {
	case string:
		content = typed
	default:
		return nil
	}
	uri := strings.TrimSpace(requestURI)
	if uri == "" {
		return nil
	}
	return &ReadEntry{
		URI:         uri,
		Title:       stripMarkdownExt(lastSegment(uri)),
		ContentType: contentTypeFromURI(uri),
		Content:     content,
	}
}

func normalizeModernWrite(fallbackURI string, body map[string]any) *WriteResult {
	resultMap, _ := anyMap(body["result"])
	if resultMap == nil && strings.EqualFold(anyString(body["status"]), "ok") {
		return &WriteResult{URI: fallbackURI, Synced: true}
	}
	uri := strings.TrimSpace(anyString(resultMap["uri"]))
	if uri == "" {
		uri = fallbackURI
	}
	semanticStatus := strings.TrimSpace(anyString(resultMap["semantic_status"]))
	vectorStatus := strings.TrimSpace(anyString(resultMap["vector_status"]))
	contentUpdated := anyBool(resultMap["content_updated"])
	// Treat queued/updated as successful write.
	synced := true
	if semanticStatus == "queued" || vectorStatus == "queued" {
		synced = false
	}
	if contentUpdated {
		synced = true
	}
	return &WriteResult{URI: uri, Synced: synced}
}

func isNotFoundError(err error) bool {
	var ovErr *Error
	if !errors.As(err, &ovErr) {
		return false
	}
	return ovErr.StatusCode == http.StatusNotFound
}

func searchLimitFromBudget(budget int) int {
	if budget <= 0 {
		return 20
	}
	limit := budget / 200
	if limit < 5 {
		limit = 5
	}
	if limit > 50 {
		limit = 50
	}
	return limit
}

func firstMapSlice(body map[string]any, keys ...string) ([]map[string]any, bool) {
	for _, key := range keys {
		raw, ok := body[key]
		if !ok {
			continue
		}
		out, ok := mapSlice(raw)
		if ok {
			return out, true
		}
	}
	return nil, false
}

func mapSlice(raw any) ([]map[string]any, bool) {
	items, ok := anySlice(raw)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry, ok := anyMap(item)
		if !ok {
			continue
		}
		out = append(out, entry)
	}
	return out, true
}

func anyMap(raw any) (map[string]any, bool) {
	switch typed := raw.(type) {
	case map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

func anySlice(raw any) ([]any, bool) {
	switch typed := raw.(type) {
	case []any:
		return typed, true
	default:
		return nil, false
	}
}

func anyString(raw any) string {
	switch typed := raw.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func firstNonEmptyAnyString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(anyString(m[key])); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func anyFloat(values ...any) float64 {
	for _, raw := range values {
		switch typed := raw.(type) {
		case float64:
			return typed
		case float32:
			return float64(typed)
		case int:
			return float64(typed)
		case int64:
			return float64(typed)
		case json.Number:
			parsed, _ := typed.Float64()
			return parsed
		}
	}
	return 0
}

func anyBool(raw any) bool {
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func inferItemType(uri string) string {
	lower := strings.ToLower(strings.TrimSpace(uri))
	switch {
	case strings.HasSuffix(lower, ".md"), strings.HasSuffix(lower, ".markdown"):
		return "markdown"
	case strings.HasSuffix(lower, ".json"):
		return "json"
	case strings.HasSuffix(lower, "/"):
		return "directory"
	case strings.HasSuffix(lower, ".txt"):
		return "text"
	default:
		return "text"
	}
}

func contentTypeFromURI(uri string) string {
	lower := strings.ToLower(strings.TrimSpace(uri))
	switch {
	case strings.HasSuffix(lower, ".md"), strings.HasSuffix(lower, ".markdown"):
		return "text/markdown"
	case strings.HasSuffix(lower, ".json"):
		return "application/json"
	default:
		return "text/plain"
	}
}

func stripMarkdownExt(name string) string {
	value := strings.TrimSpace(name)
	lower := strings.ToLower(value)
	switch {
	case len(value) >= 3 && strings.HasSuffix(lower, ".md"):
		return strings.TrimSpace(value[:len(value)-3])
	case len(value) >= 9 && strings.HasSuffix(lower, ".markdown"):
		return strings.TrimSpace(value[:len(value)-9])
	default:
		return value
	}
}

func lastSegment(uri string) string {
	trimmed := strings.TrimSpace(uri)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		parts := strings.Split(strings.Trim(trimmed, "/"), "/")
		if len(parts) == 0 {
			return ""
		}
		return parts[len(parts)-1]
	}
	pathValue := strings.Trim(parsed.Path, "/")
	if pathValue == "" {
		return strings.TrimSpace(parsed.Host)
	}
	parts := strings.Split(pathValue, "/")
	return parts[len(parts)-1]
}

func pathClean(value string) string {
	parts := make([]string, 0)
	for _, part := range strings.Split(strings.ReplaceAll(value, "\\", "/"), "/") {
		part = strings.TrimSpace(part)
		switch part {
		case "", ".":
			continue
		case "..":
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
		default:
			parts = append(parts, part)
		}
	}
	return "/" + strings.Join(parts, "/")
}

func pathJoin(parts ...string) string {
	valid := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		valid = append(valid, strings.Trim(strings.ReplaceAll(part, "\\", "/"), "/"))
	}
	return strings.Join(valid, "/")
}

func looksLikeHTMLContentType(contentType string) bool {
	contentType = strings.TrimSpace(strings.ToLower(contentType))
	return strings.Contains(contentType, "text/html")
}

func looksLikeLoginRedirect(location string) bool {
	lower := strings.ToLower(strings.TrimSpace(location))
	return strings.Contains(lower, "/login") || strings.Contains(lower, "/signin") || strings.Contains(lower, "redirect=")
}

func classifyTransportError(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &Error{Op: op, Kind: ErrorKindUnavailable, Err: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return &Error{Op: op, Kind: ErrorKindUnavailable, Err: err}
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return &Error{Op: op, Kind: ErrorKindUnavailable, Err: err}
	}
	return &Error{Op: op, Kind: ErrorKindUnexpected, Err: err}
}

func classifyStatus(statusCode int) ErrorKind {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return ErrorKindUnauthorized
	case statusCode >= 400 && statusCode < 500:
		return ErrorKindBadRequest
	case statusCode >= 500:
		return ErrorKindUnavailable
	default:
		return ErrorKindUnexpected
	}
}

func slugify(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return "untitled"
	}
	input = strings.ReplaceAll(input, " ", "-")
	input = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, input)
	if input == "" {
		return "untitled"
	}
	return input
}
