package handlers

import (
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WorkspaceFile describes a single file in the agent workspace.
type WorkspaceFile struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`       // relative to workspace root, forward-slash separated
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

func (h *Handlers) workspaceRoot() (string, error) {
	root := h.Cfg.FilesystemRoot
	if root == "" {
		root = "workspace"
	}
	return filepath.Abs(root)
}

// safePath joins root + relPath and verifies the result stays within root.
func safePath(root, relPath string) (string, error) {
	abs := filepath.Join(root, filepath.FromSlash(relPath))
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path outside workspace")
	}
	return abs, nil
}

// ListWorkspaceFiles lists files under the workspace root.
// Optional query param: since=<unix seconds> — omit files modified before that time.
// GET /api/workspace
func (h *Handlers) ListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	root, err := h.workspaceRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace unavailable")
		return
	}

	var since time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
			since = time.Unix(ts, 0)
		}
	}

	var files []WorkspaceFile
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !since.IsZero() && !info.ModTime().After(since) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		files = append(files, WorkspaceFile{
			Name:       d.Name(),
			Path:       filepath.ToSlash(rel),
			Size:       info.Size(),
			ModifiedAt: info.ModTime(),
		})
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModifiedAt.After(files[j].ModifiedAt)
	})
	if files == nil {
		files = []WorkspaceFile{}
	}
	writeJSON(w, http.StatusOK, files)
}

// GetWorkspaceFile serves a single workspace file.
// Query params: path=<relative path>, download=1 (optional, forces download).
// GET /api/workspace/file?path=hello.py
func (h *Handlers) GetWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	root, err := h.workspaceRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace unavailable")
		return
	}

	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	abs, err := safePath(root, relPath)
	if err != nil {
		writeError(w, http.StatusForbidden, "invalid path")
		return
	}

	f, err := os.Open(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusBadRequest, "not a file")
		return
	}

	ext := strings.ToLower(filepath.Ext(abs))
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)

	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(abs)+`"`)
	}

	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}
