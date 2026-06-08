package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FilesystemTool provides read/write/list/delete operations scoped to a root directory.
type FilesystemTool struct {
	root string // absolute, cleaned path
}

func NewFilesystemTool(root string) *FilesystemTool {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return &FilesystemTool{root: filepath.Clean(abs)}
}

func (t *FilesystemTool) Name() string { return "filesystem" }

func (t *FilesystemTool) Description() string {
	return "Read, write, list, or delete files within the allowed workspace directory."
}

func (t *FilesystemTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"read", "write", "list", "delete"},
				"description": "File operation to perform",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Relative path within the workspace",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "File content (for write operation)",
			},
		},
		"required": []string{"operation", "path"},
	}
}

func (t *FilesystemTool) Execute(_ context.Context, input map[string]any) (ToolResult, error) {
	op, _ := input["operation"].(string)
	relPath, _ := input["path"].(string)

	absPath, err := t.resolve(relPath)
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	switch op {
	case "read":
		data, err := os.ReadFile(absPath)
		if err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("read %s: %s", relPath, err)}, nil
		}
		return ToolResult{Content: string(data)}, nil

	case "write":
		content, _ := input["content"].(string)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("mkdir: %s", err)}, nil
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("write %s: %s", relPath, err)}, nil
		}
		return ToolResult{Content: fmt.Sprintf("wrote %d bytes to %s", len(content), relPath)}, nil

	case "list":
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("list %s: %s", relPath, err)}, nil
		}
		var sb strings.Builder
		for _, e := range entries {
			if e.IsDir() {
				fmt.Fprintf(&sb, "%s/\n", e.Name())
			} else {
				info, _ := e.Info()
				if info != nil {
					fmt.Fprintf(&sb, "%s (%d bytes)\n", e.Name(), info.Size())
				} else {
					fmt.Fprintf(&sb, "%s\n", e.Name())
				}
			}
		}
		return ToolResult{Content: sb.String()}, nil

	case "delete":
		if err := os.Remove(absPath); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("delete %s: %s", relPath, err)}, nil
		}
		return ToolResult{Content: fmt.Sprintf("deleted %s", relPath)}, nil

	default:
		return ToolResult{IsError: true, Content: fmt.Sprintf("unknown operation %q", op)}, nil
	}
}

// resolve returns the absolute path after validating it stays within root.
func (t *FilesystemTool) resolve(relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("path is required")
	}
	abs := filepath.Clean(filepath.Join(t.root, relPath))
	if !strings.HasPrefix(abs, t.root+string(filepath.Separator)) && abs != t.root {
		return "", fmt.Errorf("path %q escapes workspace root", relPath)
	}
	return abs, nil
}
