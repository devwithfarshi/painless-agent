package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemTool_TraversalRejected(t *testing.T) {
	root := t.TempDir()
	tool := NewFilesystemTool(root)
	ctx := context.Background()

	cases := []string{
		"../secret",
		"../../etc/passwd",
		"subdir/../../outside",
	}
	for _, p := range cases {
		res, err := tool.Execute(ctx, map[string]any{"operation": "read", "path": p})
		if err != nil {
			t.Fatalf("path %q: unexpected Go error: %v", p, err)
		}
		if !res.IsError {
			t.Errorf("path %q: expected IsError=true, got content=%q", p, res.Content)
		}
	}
}

func TestFilesystemTool_WriteAndRead(t *testing.T) {
	root := t.TempDir()
	tool := NewFilesystemTool(root)
	ctx := context.Background()

	content := "hello, agent"
	res, err := tool.Execute(ctx, map[string]any{
		"operation": "write",
		"path":      "notes.txt",
		"content":   content,
	})
	if err != nil || res.IsError {
		t.Fatalf("write failed: err=%v isErr=%v content=%q", err, res.IsError, res.Content)
	}

	res, err = tool.Execute(ctx, map[string]any{"operation": "read", "path": "notes.txt"})
	if err != nil || res.IsError {
		t.Fatalf("read failed: err=%v isErr=%v content=%q", err, res.IsError, res.Content)
	}
	if res.Content != content {
		t.Errorf("got %q, want %q", res.Content, content)
	}
}

func TestFilesystemTool_WriteCreatesSubdirectories(t *testing.T) {
	root := t.TempDir()
	tool := NewFilesystemTool(root)
	ctx := context.Background()

	res, err := tool.Execute(ctx, map[string]any{
		"operation": "write",
		"path":      "deep/nested/file.txt",
		"content":   "nested content",
	})
	if err != nil || res.IsError {
		t.Fatalf("write nested failed: err=%v isErr=%v content=%q", err, res.IsError, res.Content)
	}
	if _, err := os.Stat(filepath.Join(root, "deep/nested/file.txt")); err != nil {
		t.Fatalf("file not created on disk: %v", err)
	}
}

func TestFilesystemTool_List(t *testing.T) {
	root := t.TempDir()
	tool := NewFilesystemTool(root)
	ctx := context.Background()

	// Write two files
	for _, name := range []string{"a.txt", "b.txt"} {
		res, _ := tool.Execute(ctx, map[string]any{"operation": "write", "path": name, "content": name})
		if res.IsError {
			t.Fatalf("write %s: %s", name, res.Content)
		}
	}

	res, err := tool.Execute(ctx, map[string]any{"operation": "list", "path": "."})
	if err != nil || res.IsError {
		t.Fatalf("list failed: err=%v content=%q", err, res.Content)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if !contains(res.Content, name) {
			t.Errorf("list output missing %q: %s", name, res.Content)
		}
	}
}

func TestFilesystemTool_Delete(t *testing.T) {
	root := t.TempDir()
	tool := NewFilesystemTool(root)
	ctx := context.Background()

	tool.Execute(ctx, map[string]any{"operation": "write", "path": "del.txt", "content": "bye"}) //nolint

	res, err := tool.Execute(ctx, map[string]any{"operation": "delete", "path": "del.txt"})
	if err != nil || res.IsError {
		t.Fatalf("delete failed: err=%v content=%q", err, res.Content)
	}
	if _, statErr := os.Stat(filepath.Join(root, "del.txt")); !os.IsNotExist(statErr) {
		t.Error("file still exists after delete")
	}
}

func TestFilesystemTool_UnknownOperation(t *testing.T) {
	root := t.TempDir()
	tool := NewFilesystemTool(root)
	ctx := context.Background()

	res, err := tool.Execute(ctx, map[string]any{"operation": "chmod", "path": "x.txt"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for unknown operation")
	}
}

func TestFilesystemTool_EmptyPath(t *testing.T) {
	root := t.TempDir()
	tool := NewFilesystemTool(root)
	ctx := context.Background()

	res, err := tool.Execute(ctx, map[string]any{"operation": "read", "path": ""})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for empty path")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})())
}
