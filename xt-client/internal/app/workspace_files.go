package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"xiaotao/xt-client/internal/desktop"
)

const (
	workspaceListLimit    = 2000
	workspacePreviewLimit = 256 << 10
	workspaceFileLimit    = 8 << 20
	workspaceImageLimit   = 20 << 20
	workspaceURLLimit     = 2048
)

// openExternalURL is a package-level seam so tests can verify URL validation
// without launching a real browser. Production always uses desktop.Browser.
var openExternalURL = desktop.Browser

var workspaceImageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
}

type workspaceRequestError struct {
	status  int
	message string
}

func (e *workspaceRequestError) Error() string { return e.message }

func workspaceBadRequest(message string) error {
	return &workspaceRequestError{status: http.StatusBadRequest, message: message}
}

func workspaceNotFound(message string) error {
	return &workspaceRequestError{status: http.StatusNotFound, message: message}
}

type workspaceEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Dir      bool   `json:"dir"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

type workspaceListing struct {
	Root      string           `json:"root"`
	Path      string           `json:"path"`
	Parent    string           `json:"parent"`
	Truncated bool             `json:"truncated"`
	Entries   []workspaceEntry `json:"entries"`
}

type workspaceFilePreview struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Modified  string `json:"modified"`
	Kind      string `json:"kind"`
	Truncated bool   `json:"truncated"`
	Content   string `json:"content"`
}

// registerWorkspaceRoutes exposes project-scoped file browsing and external
// link opening for the client workspace tabs. All routes are same-origin
// checked like the rest of the management API.
func registerWorkspaceRoutes(mux *http.ServeMux, requireOrigin func(*http.Request) bool, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	guard := func(w http.ResponseWriter, r *http.Request) bool {
		if !requireOrigin(r) {
			http.Error(w, "same-origin required", 403)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/workspace/files", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		handleWorkspaceList(w, r, send, apiError)
	})
	mux.HandleFunc("GET /api/workspace/file", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		handleWorkspaceFilePreview(w, r, send, apiError)
	})
	mux.HandleFunc("GET /api/workspace/file/raw", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		handleWorkspaceFileRaw(w, r, apiError)
	})
	mux.HandleFunc("POST /api/system/open-url", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		handleOpenExternalURL(w, r, send, apiError)
	})
}

func handleWorkspaceList(w http.ResponseWriter, r *http.Request, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	root, rel, target, err := resolveWorkspacePath(r.URL.Query().Get("projectPath"), r.URL.Query().Get("path"))
	if err != nil {
		writeWorkspaceError(w, err, apiError)
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		writeWorkspaceError(w, workspaceNotFound("文件或目录不存在"), apiError)
		return
	}
	if !info.IsDir() {
		apiError(w, http.StatusBadRequest, "path 不是目录")
		return
	}
	dirEntries, err := os.ReadDir(target)
	if err != nil {
		apiError(w, http.StatusBadRequest, "无法读取目录: "+err.Error())
		return
	}
	entries := make([]workspaceEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entries = append(entries, workspaceEntry{
			Name:     entry.Name(),
			Path:     path.Join(rel, entry.Name()),
			Dir:      info.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Dir != entries[j].Dir {
			return entries[i].Dir
		}
		left, right := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if left != right {
			return left < right
		}
		return entries[i].Name < entries[j].Name
	})
	truncated := len(entries) > workspaceListLimit
	if truncated {
		entries = entries[:workspaceListLimit]
	}
	send(w, workspaceListing{
		Root:      root,
		Path:      rel,
		Parent:    workspaceParent(rel),
		Truncated: truncated,
		Entries:   entries,
	})
}

func handleWorkspaceFilePreview(w http.ResponseWriter, r *http.Request, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	_, rel, target, err := resolveWorkspacePath(r.URL.Query().Get("projectPath"), r.URL.Query().Get("path"))
	if err != nil {
		writeWorkspaceError(w, err, apiError)
		return
	}
	if rel == "" {
		apiError(w, http.StatusBadRequest, "path 必须是文件")
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		writeWorkspaceError(w, workspaceNotFound("文件不存在"), apiError)
		return
	}
	if info.IsDir() {
		apiError(w, http.StatusBadRequest, "path 是目录")
		return
	}
	preview := workspaceFilePreview{
		Path:     rel,
		Name:     filepath.Base(target),
		Size:     info.Size(),
		Modified: info.ModTime().UTC().Format(time.RFC3339),
		Kind:     "text",
	}
	if info.Size() > workspaceFileLimit {
		preview.Kind, preview.Truncated = "too-large", true
		send(w, preview)
		return
	}
	if workspaceImageTypes[strings.ToLower(filepath.Ext(target))] != "" {
		preview.Kind = "image"
		send(w, preview)
		return
	}
	file, err := os.Open(target)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "无法打开文件")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, workspacePreviewLimit))
	if err != nil {
		apiError(w, http.StatusInternalServerError, "读取文件失败")
		return
	}
	preview.Truncated = info.Size() > int64(len(content))
	if bytes.IndexByte(content, 0) >= 0 {
		preview.Kind = "binary"
	} else {
		preview.Content = string(content)
	}
	send(w, preview)
}

func handleWorkspaceFileRaw(w http.ResponseWriter, r *http.Request, apiError func(http.ResponseWriter, int, string)) {
	_, rel, target, err := resolveWorkspacePath(r.URL.Query().Get("projectPath"), r.URL.Query().Get("path"))
	if err != nil {
		writeWorkspaceError(w, err, apiError)
		return
	}
	if rel == "" {
		apiError(w, http.StatusBadRequest, "path 必须是图片文件")
		return
	}
	contentType := workspaceImageTypes[strings.ToLower(filepath.Ext(target))]
	if contentType == "" {
		apiError(w, http.StatusBadRequest, "只允许图片文件")
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		writeWorkspaceError(w, workspaceNotFound("文件不存在"), apiError)
		return
	}
	if info.IsDir() {
		apiError(w, http.StatusBadRequest, "path 是目录")
		return
	}
	if info.Size() > workspaceImageLimit {
		apiError(w, http.StatusBadRequest, "图片超过 20MiB")
		return
	}
	file, err := os.Open(target)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "无法打开文件")
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

func handleOpenExternalURL(w http.ResponseWriter, r *http.Request, send func(http.ResponseWriter, any), apiError func(http.ResponseWriter, int, string)) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&input); err != nil {
		apiError(w, http.StatusBadRequest, "请求格式无效")
		return
	}
	target, err := validExternalURL(input.URL)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := openExternalURL(target); err != nil {
		apiError(w, http.StatusInternalServerError, "无法打开链接: "+err.Error())
		return
	}
	send(w, map[string]bool{"ok": true})
}

func validExternalURL(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("url 不能为空")
	}
	if len(raw) > workspaceURLLimit {
		return "", fmt.Errorf("url 超过 %d 个字符", workspaceURLLimit)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("url 格式无效")
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("只允许 http/https 链接")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("url 缺少主机名")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("url 不能包含用户信息")
	}
	return parsed.String(), nil
}

// resolveWorkspacePath resolves a project-relative path and rejects absolute
// paths and symlink escapes. Both the project root and target are resolved
// before the containment check so links cannot point outside the root.
func resolveWorkspacePath(projectPath, rawPath string) (string, string, string, error) {
	projectPath = strings.TrimSpace(projectPath)
	if projectPath == "" {
		return "", "", "", workspaceBadRequest("缺少 projectPath")
	}
	rootAbs, err := filepath.Abs(projectPath)
	if err != nil {
		return "", "", "", workspaceBadRequest("projectPath 无效")
	}
	rootInfo, err := os.Stat(rootAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", "", workspaceBadRequest("项目目录不存在")
		}
		return "", "", "", workspaceBadRequest("无法访问项目目录")
	}
	if !rootInfo.IsDir() {
		return "", "", "", workspaceBadRequest("projectPath 不是目录")
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", "", "", workspaceBadRequest("无法解析项目目录")
	}
	rootAbs = filepath.Clean(resolvedRoot)

	if filepath.IsAbs(rawPath) || filepath.VolumeName(rawPath) != "" {
		return "", "", "", workspaceBadRequest("path 必须是相对路径")
	}
	for _, part := range strings.FieldsFunc(rawPath, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return "", "", "", workspaceBadRequest("path 不能包含 ..")
		}
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(rawPath)))
	if err != nil {
		return "", "", "", workspaceBadRequest("path 无效")
	}
	resolvedTarget, err := filepath.EvalSymlinks(targetAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", "", workspaceNotFound("文件或目录不存在")
		}
		return "", "", "", workspaceBadRequest("无法解析目标路径")
	}
	rel, err := filepath.Rel(rootAbs, resolvedTarget)
	if err != nil {
		return "", "", "", workspaceBadRequest("path 无法解析为项目内路径")
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", "", workspaceBadRequest("path 超出项目目录")
	}
	if rel == "." {
		rel = ""
	}
	return rootAbs, filepath.ToSlash(rel), resolvedTarget, nil
}

func workspaceParent(rel string) string {
	if rel == "" {
		return ""
	}
	parent := path.Dir(rel)
	if parent == "." {
		return ""
	}
	return parent
}

func writeWorkspaceError(w http.ResponseWriter, err error, apiError func(http.ResponseWriter, int, string)) {
	status := http.StatusBadRequest
	var requestErr *workspaceRequestError
	if errors.As(err, &requestErr) {
		status = requestErr.status
	}
	apiError(w, status, err.Error())
}
