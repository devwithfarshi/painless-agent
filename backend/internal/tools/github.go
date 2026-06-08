package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	gogithttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogithub "github.com/google/go-github/v73/github"
)

// GitHubTool creates repositories and commits/pushes files via the GitHub API + go-git.
type GitHubTool struct {
	token  string
	client *gogithub.Client
}

func NewGitHubTool(token string) *GitHubTool {
	return &GitHubTool{
		token:  token,
		client: gogithub.NewClient(nil).WithAuthToken(token),
	}
}

func (t *GitHubTool) Name() string { return "github" }

func (t *GitHubTool) Description() string {
	return "Interact with GitHub: create a repository or commit files and push them to an existing repo."
}

func (t *GitHubTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"create_repo", "commit_and_push"},
				"description": "Operation to perform",
			},
			"owner": map[string]any{
				"type":        "string",
				"description": "GitHub username or org (required for commit_and_push; inferred for create_repo)",
			},
			"repo": map[string]any{
				"type":        "string",
				"description": "Repository name (required)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Repo description (for create_repo)",
			},
			"private": map[string]any{
				"type":        "boolean",
				"description": "Create a private repo (default: false)",
			},
			"files": map[string]any{
				"type":        "array",
				"description": "Files to write and commit (for commit_and_push)",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string", "description": "Relative file path"},
						"content": map[string]any{"type": "string", "description": "File content"},
					},
					"required": []string{"path", "content"},
				},
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Commit message (for commit_and_push)",
			},
		},
		"required": []string{"operation", "repo"},
	}
}

func (t *GitHubTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	op, _ := input["operation"].(string)
	switch op {
	case "create_repo":
		return t.createRepo(ctx, input)
	case "commit_and_push":
		return t.commitAndPush(ctx, input)
	default:
		return ToolResult{IsError: true, Content: fmt.Sprintf("unknown operation %q", op)}, nil
	}
}

func (t *GitHubTool) createRepo(ctx context.Context, input map[string]any) (ToolResult, error) {
	name, _ := input["repo"].(string)
	desc, _ := input["description"].(string)
	private, _ := input["private"].(bool)

	if name == "" {
		return ToolResult{IsError: true, Content: "repo is required"}, nil
	}

	repo, _, err := t.client.Repositories.Create(ctx, "", &gogithub.Repository{
		Name:        gogithub.String(name),
		Description: gogithub.String(desc),
		Private:     gogithub.Bool(private),
		AutoInit:    gogithub.Bool(true),
	})
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("create repo: %s", err)}, nil
	}

	return ToolResult{Content: fmt.Sprintf("created repository %s at %s", name, repo.GetHTMLURL())}, nil
}

func (t *GitHubTool) commitAndPush(ctx context.Context, input map[string]any) (ToolResult, error) {
	owner, _ := input["owner"].(string)
	repoName, _ := input["repo"].(string)
	message, _ := input["message"].(string)
	rawFiles, _ := input["files"].([]any)

	if repoName == "" {
		return ToolResult{IsError: true, Content: "repo is required"}, nil
	}
	if len(rawFiles) == 0 {
		return ToolResult{IsError: true, Content: "files is required for commit_and_push"}, nil
	}
	if message == "" {
		message = "commit by painless-agent"
	}

	// Infer owner from authenticated user if not provided.
	if owner == "" {
		user, _, err := t.client.Users.Get(ctx, "")
		if err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("get authenticated user: %s", err)}, nil
		}
		owner = user.GetLogin()
	}

	tmpDir, err := os.MkdirTemp("", "github-push-*")
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("create tmpdir: %s", err)}, nil
	}
	defer os.RemoveAll(tmpDir)

	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repoName)
	auth := &gogithttp.BasicAuth{Username: "x-token", Password: t.token}

	repo, err := git.PlainCloneContext(ctx, tmpDir, false, &git.CloneOptions{
		URL:   cloneURL,
		Auth:  auth,
		Depth: 1,
	})
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("clone %s: %s", cloneURL, err)}, nil
	}

	wt, err := repo.Worktree()
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("get worktree: %s", err)}, nil
	}

	var staged []string
	for _, rf := range rawFiles {
		fm, ok := rf.(map[string]any)
		if !ok {
			continue
		}
		relPath, _ := fm["path"].(string)
		content, _ := fm["content"].(string)
		if relPath == "" {
			continue
		}

		// Resolve against tmpDir, reject any traversal attempts.
		abs := filepath.Join(tmpDir, filepath.Clean("/"+relPath))
		if !strings.HasPrefix(abs, tmpDir+string(filepath.Separator)) {
			return ToolResult{IsError: true, Content: fmt.Sprintf("path %q escapes repo root", relPath)}, nil
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("mkdir: %s", err)}, nil
		}
		if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("write %s: %s", relPath, err)}, nil
		}
		if _, err := wt.Add(relPath); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("git add %s: %s", relPath, err)}, nil
		}
		staged = append(staged, relPath)
	}

	if len(staged) == 0 {
		return ToolResult{IsError: true, Content: "no valid files to commit"}, nil
	}

	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "painless-agent",
			Email: "agent@painless-agent.local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("commit: %s", err)}, nil
	}

	if err := repo.PushContext(ctx, &git.PushOptions{Auth: auth}); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("push: %s", err)}, nil
	}

	return ToolResult{
		Content: fmt.Sprintf("committed %d file(s) to %s/%s — %s (sha: %s)",
			len(staged), owner, repoName, message, hash.String()),
	}, nil
}
