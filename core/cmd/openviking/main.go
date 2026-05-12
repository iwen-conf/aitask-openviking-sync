package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultHost    = "0.0.0.0"
	defaultPort    = "9090"
	defaultDataDir = "/data"
)

type listItem struct {
	Title string `json:"title"`
	URI   string `json:"uri"`
	Type  string `json:"type"`
}

type searchHit struct {
	URI             string `json:"uri"`
	Title           string `json:"title"`
	Snippet         string `json:"snippet"`
	EstimatedTokens int    `json:"estimatedTokens,omitempty"`
}

type readEntry struct {
	URI         string `json:"uri"`
	Title       string `json:"title"`
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type writeRequest struct {
	Target        string `json:"target"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	RelatedTaskID string `json:"relatedTaskId"`
}

type writeResponse struct {
	URI    string `json:"uri"`
	Synced bool   `json:"synced"`
}

type server struct {
	dataDir string
}

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "--healthcheck" {
		os.Exit(runHealthcheck(os.Args[2]))
	}

	srv := &server{dataDir: getenv("OPENVIKING_DATA_DIR", defaultDataDir)}
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleRoot)
	mux.HandleFunc("/healthz", srv.handleRoot)
	mux.HandleFunc("/api/v1/namespaces/", srv.handleAPI)

	addr := getenv("OPENVIKING_HOST", defaultHost) + ":" + getenv("OPENVIKING_PORT", defaultPort)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("openviking mock listening on %s data=%s", addr, srv.dataDir)
	log.Fatal(httpServer.ListenAndServe())
}

func runHealthcheck(target string) int {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return 1
	}
	resp, err := client.Do(req)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return 1
	}
	return 0
}

func (s *server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "openviking-mock",
		"status":  "ok",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *server) handleAPI(w http.ResponseWriter, r *http.Request) {
	namespace, projectID, action, err := parseAPIPath(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "memory":
		s.handleList(w, namespace, projectID)
	case r.Method == http.MethodGet && action == "memory/search":
		s.handleSearch(w, r, namespace, projectID)
	case r.Method == http.MethodGet && action == "memory/read":
		s.handleRead(w, r, namespace, projectID)
	case r.Method == http.MethodPost && action == "memory/write":
		s.handleWrite(w, r, namespace, projectID)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unsupported endpoint"))
	}
}

func parseAPIPath(raw string) (string, string, string, error) {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 7 {
		return "", "", "", fmt.Errorf("invalid path")
	}
	if parts[0] != "api" || parts[1] != "v1" || parts[2] != "namespaces" || parts[4] != "projects" {
		return "", "", "", fmt.Errorf("invalid path")
	}
	if parts[6] != "memory" {
		return "", "", "", fmt.Errorf("invalid path")
	}
	namespace := strings.TrimSpace(parts[3])
	projectID := strings.TrimSpace(parts[5])
	action := "memory"
	if len(parts) > 7 {
		action += "/" + strings.Join(parts[7:], "/")
	}
	if namespace == "" || projectID == "" {
		return "", "", "", fmt.Errorf("invalid path")
	}
	return namespace, projectID, action, nil
}

func (s *server) handleList(w http.ResponseWriter, namespace string, projectID string) {
	items, err := s.listProjectFiles(namespace, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request, namespace string, projectID string) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	budget, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("budget")))
	refsOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("refsOnly")), "true")

	items, err := s.listProjectFiles(namespace, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	results := make([]searchHit, 0)
	totalBudget := 0
	for _, item := range items {
		entry, err := s.readByURI(namespace, projectID, item.URI)
		if err != nil {
			continue
		}
		if query != "" && !matchesSearch(entry.Title, entry.Content, query) {
			continue
		}
		estimatedTokens := estimateTokens(entry.Content)
		if budget > 0 && totalBudget+estimatedTokens > budget && len(results) > 0 {
			break
		}
		totalBudget += estimatedTokens
		result := searchHit{
			URI:             entry.URI,
			Title:           entry.Title,
			EstimatedTokens: estimatedTokens,
		}
		if !refsOnly {
			result.Snippet = makeSnippet(entry.Content, query)
		}
		results = append(results, result)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": results})
}

func (s *server) handleRead(w http.ResponseWriter, r *http.Request, namespace string, projectID string) {
	uri := strings.TrimSpace(r.URL.Query().Get("uri"))
	if uri == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("uri is required"))
		return
	}
	entry, err := s.readByURI(namespace, projectID, uri)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *server) handleWrite(w http.ResponseWriter, r *http.Request, namespace string, projectID string) {
	defer r.Body.Close()

	var req writeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid json: %w", err))
		return
	}
	target := strings.TrimSpace(strings.ToLower(req.Target))
	title := strings.TrimSpace(req.Title)
	content := req.Content
	if target == "" || title == "" || strings.TrimSpace(content) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("target/title/content cannot be empty"))
		return
	}

	root := s.projectRoot(namespace, projectID)
	if err := ensureScaffold(root); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	rel, err := resolveWritePath(target, title)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	targetFile := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(targetFile), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(targetFile, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, writeResponse{
		URI:    buildURI(namespace, projectID, rel),
		Synced: true,
	})
}

func (s *server) listProjectFiles(namespace string, projectID string) ([]listItem, error) {
	root := s.projectRoot(namespace, projectID)
	if err := ensureScaffold(root); err != nil {
		return nil, err
	}
	items := make([]listItem, 0)
	err := filepath.WalkDir(root, func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		items = append(items, listItem{
			Title: extractTitle(d.Name(), string(content)),
			URI:   buildURI(namespace, projectID, filepath.ToSlash(rel)),
			Type:  "markdown",
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *server) readByURI(namespace string, projectID string, uri string) (readEntry, error) {
	rel, err := parseURI(namespace, projectID, uri)
	if err != nil {
		return readEntry{}, err
	}
	filePath := filepath.Join(s.projectRoot(namespace, projectID), filepath.FromSlash(rel))
	content, err := os.ReadFile(filePath)
	if err != nil {
		return readEntry{}, err
	}
	return readEntry{
		URI:         buildURI(namespace, projectID, rel),
		Title:       extractTitle(filepath.Base(filePath), string(content)),
		ContentType: "text/markdown",
		Content:     string(content),
	}, nil
}

func (s *server) projectRoot(namespace string, projectID string) string {
	return filepath.Join(s.dataDir, "namespaces", namespace, "projects", projectID)
}

func ensureScaffold(root string) error {
	dirs := []string{
		"brief",
		"memory/decisions",
		"memory/summaries",
		"memory/agent-experience",
		"memory/room",
		"memory/handoffs",
		"memory/mistakes",
		"memory/notes",
		"memory/reports",
		"resources/api",
		"resources/database",
		"resources/cli",
		"resources/frontend",
		"skills",
		"tasks",
		"sessions",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func resolveWritePath(target string, title string) (string, error) {
	base := ""
	switch strings.TrimSpace(strings.ToLower(target)) {
	case "brief":
		base = "brief"
	case "memory":
		base = "memory"
	case "resource", "resources":
		base = "resources"
	case "skill", "skills":
		base = "skills"
	case "tasks":
		base = "tasks"
	case "sessions":
		base = "sessions"
	case "decision", "decisions":
		base = "memory/decisions"
	case "summary":
		base = "memory/summaries"
	case "handoff", "handoffs":
		base = "memory/handoffs"
	case "note":
		base = "memory/notes"
	case "report":
		base = "memory/reports"
	default:
		return "", fmt.Errorf("unsupported target %q", target)
	}
	relTitle := normalizeTitlePath(title)
	if relTitle == "" {
		relTitle = "untitled.md"
	}
	return path.Join(base, relTitle), nil
}

func normalizeTitlePath(title string) string {
	cleaned := strings.ReplaceAll(strings.TrimSpace(title), "\\", "/")
	cleaned = path.Clean("/" + cleaned)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." || cleaned == "" {
		return ""
	}
	parts := strings.Split(cleaned, "/")
	sanitized := make([]string, 0, len(parts))
	for i, part := range parts {
		part = sanitizeSegment(part)
		if part == "" {
			part = "untitled"
		}
		if i == len(parts)-1 && !strings.HasSuffix(strings.ToLower(part), ".md") {
			part += ".md"
		}
		sanitized = append(sanitized, part)
	}
	return path.Join(sanitized...)
}

func sanitizeSegment(input string) string {
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

func parseURI(namespace string, projectID string, raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "viking" {
		return "", fmt.Errorf("invalid uri scheme")
	}
	if strings.TrimSpace(parsed.Host) != namespace {
		return "", fmt.Errorf("namespace mismatch")
	}
	prefix := "/projects/" + projectID + "/"
	if !strings.HasPrefix(parsed.Path, prefix) {
		return "", fmt.Errorf("project mismatch")
	}
	rel := strings.TrimPrefix(parsed.Path, prefix)
	rel = path.Clean("/" + rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." {
		return "", fmt.Errorf("invalid uri path")
	}
	return rel, nil
}

func buildURI(namespace string, projectID string, rel string) string {
	return "viking://" + namespace + "/projects/" + projectID + "/" + strings.TrimPrefix(filepath.ToSlash(rel), "/")
}

func matchesSearch(title string, content string, query string) bool {
	haystack := strings.ToLower(title + "\n" + content)
	for _, token := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func makeSnippet(content string, query string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	if query == "" {
		return truncate(trimmed, 180)
	}
	lowerContent := strings.ToLower(trimmed)
	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	index := strings.Index(lowerContent, lowerQuery)
	if index < 0 {
		return truncate(trimmed, 180)
	}
	start := index - 80
	if start < 0 {
		start = 0
	}
	end := index + len(lowerQuery) + 80
	if end > len(trimmed) {
		end = len(trimmed)
	}
	return truncate(strings.TrimSpace(trimmed[start:end]), 180)
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

func estimateTokens(content string) int {
	size := utf8.RuneCountInString(strings.TrimSpace(content))
	if size <= 0 {
		return 1
	}
	estimated := size / 4
	if estimated <= 0 {
		return 1
	}
	return estimated
}

func extractTitle(fileName string, content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	writeJSON(w, statusCode, map[string]any{
		"error": strings.TrimSpace(err.Error()),
	})
}

func getenv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
