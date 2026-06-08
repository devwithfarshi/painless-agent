package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devwithfarshi/painless-agent/internal/sandbox"
)

// langConfig maps a language name to its sandbox image, file extension, and command builder.
type langConfig struct {
	image string
	ext   string
	cmd   func(containerPath string) []string
}

// supportedLanguages is the fixed allowlist — image names are never derived from user input.
var supportedLanguages = map[string]langConfig{
	"python": {
		image: "python:3.12-slim",
		ext:   ".py",
		cmd:   func(f string) []string { return []string{"python3", f} },
	},
	"javascript": {
		image: "node:20-alpine",
		ext:   ".js",
		cmd:   func(f string) []string { return []string{"node", f} },
	},
	"go": {
		image: "golang:1.22-alpine",
		ext:   ".go",
		cmd:   func(f string) []string { return []string{"go", "run", f} },
	},
	"bash": {
		image: "alpine:3.19",
		ext:   ".sh",
		cmd:   func(f string) []string { return []string{"sh", f} },
	},
	"ruby": {
		image: "ruby:3.3-slim",
		ext:   ".rb",
		cmd:   func(f string) []string { return []string{"ruby", f} },
	},
}

// CodeExecutorTool runs code in an ephemeral, air-gapped Docker container.
// The image name is always derived from the internal allowlist, never from user input.
type CodeExecutorTool struct {
	runner  *sandbox.Runner
	timeout time.Duration
}

func NewCodeExecutorTool(runner *sandbox.Runner, execTimeoutSecs int) *CodeExecutorTool {
	if execTimeoutSecs <= 0 {
		execTimeoutSecs = 30
	}
	return &CodeExecutorTool{
		runner:  runner,
		timeout: time.Duration(execTimeoutSecs) * time.Second,
	}
}

func (t *CodeExecutorTool) Name() string { return "code_executor" }

func (t *CodeExecutorTool) Description() string {
	return "Execute code in an isolated Docker sandbox (network disabled, 256 MiB RAM, 0.5 vCPU). Returns stdout/stderr. Supported languages: python, javascript, go, bash, ruby."
}

func (t *CodeExecutorTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"language": map[string]any{
				"type":        "string",
				"enum":        []string{"python", "javascript", "go", "bash", "ruby"},
				"description": "Programming language for the code",
			},
			"code": map[string]any{
				"type":        "string",
				"description": "Source code to execute",
			},
		},
		"required": []string{"language", "code"},
	}
}

func (t *CodeExecutorTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	lang, _ := input["language"].(string)
	code, _ := input["code"].(string)

	lang = strings.ToLower(strings.TrimSpace(lang))
	lc, ok := supportedLanguages[lang]
	if !ok {
		langs := make([]string, 0, len(supportedLanguages))
		for k := range supportedLanguages {
			langs = append(langs, k)
		}
		return ToolResult{IsError: true, Content: fmt.Sprintf("unsupported language %q (supported: %s)", lang, strings.Join(langs, ", "))}, nil
	}
	if strings.TrimSpace(code) == "" {
		return ToolResult{IsError: true, Content: "code is required"}, nil
	}

	// Write code to a temp directory on the host; mount it read-only into the container.
	tmpDir, err := os.MkdirTemp("", "sandbox-*")
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("create tmpdir: %s", err)}, nil
	}
	defer os.RemoveAll(tmpDir)

	codeFile := "code" + lc.ext
	if err := os.WriteFile(filepath.Join(tmpDir, codeFile), []byte(code), 0644); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("write code: %s", err)}, nil
	}

	containerCodePath := "/code/" + codeFile
	containerID, err := t.runner.Run(ctx, sandbox.RunOpts{
		Image:  lc.image,
		Cmd:    lc.cmd(containerCodePath),
		Mounts: []sandbox.MountSpec{{HostPath: tmpDir, ContainerPath: "/code", ReadOnly: true}},
	})
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("launch sandbox: %s", err)}, nil
	}
	// Always clean up, even if the context is cancelled.
	defer t.runner.Remove(context.Background(), containerID)

	if err := t.runner.Wait(ctx, containerID, t.timeout); err != nil {
		// Try to capture any partial output before reporting the timeout.
		partial, _ := t.runner.Logs(ctx, containerID)
		msg := fmt.Sprintf("sandbox timed out after %s", t.timeout)
		if partial != "" {
			msg += "\npartial output:\n" + partial
		}
		return ToolResult{IsError: true, Content: msg}, nil
	}

	logs, err := t.runner.Logs(ctx, containerID)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("read output: %s", err)}, nil
	}
	if logs == "" {
		logs = "(no output)"
	}
	return ToolResult{Content: logs}, nil
}
