package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// BrowserTool controls a headless Chromium instance via chromedp.
// Each Execute call gets its own browser tab; the underlying Chrome process is shared.
//
// Two modes:
//   - Local exec (cdpURL == ""): spawns a local Chrome/Chromium process.
//   - Remote CDP (cdpURL != ""): connects to an existing Chrome via DevTools Protocol
//     (e.g. the docker-compose `chrome` service on ws://localhost:9222).
//
// chromedp's built-in modifyURL automatically calls /json/version on the remote host
// to resolve the full browser WebSocket URL, so a plain ws://host:port is enough.
type BrowserTool struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	workspace   string
	mode        string // "local" or "remote:<url>"
}

// NewBrowserTool creates a BrowserTool.
// Pass cdpURL = "" to use a local Chrome installation.
// Pass cdpURL = "ws://localhost:9222" (or http://) to use a remote Chrome container.
func NewBrowserTool(workspace, cdpURL string) *BrowserTool {
	var allocCtx context.Context
	var allocCancel context.CancelFunc
	mode := "local"

	if cdpURL != "" {
		// Remote Chrome — chromedp resolves the full browser WebSocket URL automatically.
		allocCtx, allocCancel = chromedp.NewRemoteAllocator(context.Background(), cdpURL)
		mode = "remote:" + cdpURL
	} else {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("disable-dev-shm-usage", true),
		)
		allocCtx, allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	}

	return &BrowserTool{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		workspace:   workspace,
		mode:        mode,
	}
}

// Close shuts down the allocator. For local mode this kills the Chrome process;
// for remote mode it just releases the context (the Docker container keeps running).
func (t *BrowserTool) Close() {
	if t.allocCancel != nil {
		t.allocCancel()
	}
}

func (t *BrowserTool) Name() string { return "browser" }

func (t *BrowserTool) Description() string {
	return "Control a headless browser: navigate to a URL and extract page text, take a screenshot, or click an element. Each call is stateless — include the URL in every request."
}

func (t *BrowserTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"get_text", "screenshot", "click"},
				"description": "Browser operation: get_text returns visible page text, screenshot saves a PNG, click clicks a CSS selector",
			},
			"url": map[string]any{
				"type":        "string",
				"description": "URL to navigate to before performing the operation",
			},
			"selector": map[string]any{
				"type":        "string",
				"description": "CSS selector (required for click; optional for get_text to scope extraction)",
			},
			"wait_selector": map[string]any{
				"type":        "string",
				"description": "CSS selector to wait for before acting (optional)",
			},
		},
		"required": []string{"operation", "url"},
	}
}

func (t *BrowserTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	op, _ := input["operation"].(string)
	url, _ := input["url"].(string)
	selector, _ := input["selector"].(string)
	waitSel, _ := input["wait_selector"].(string)

	if url == "" {
		return ToolResult{IsError: true, Content: "url is required"}, nil
	}

	// Each Execute creates a new tab from the shared allocator.
	tabCtx, tabCancel := chromedp.NewContext(t.allocCtx)
	defer tabCancel()

	taskCtx, taskCancel := context.WithTimeout(tabCtx, 45*time.Second)
	defer taskCancel()

	// Build the pre-action sequence: navigate, then optionally wait.
	actions := []chromedp.Action{chromedp.Navigate(url)}
	if waitSel != "" {
		actions = append(actions, chromedp.WaitVisible(waitSel, chromedp.ByQuery))
	}

	switch op {
	case "get_text":
		target := "body"
		if selector != "" {
			target = selector
		}
		var text string
		actions = append(actions,
			chromedp.WaitVisible(target, chromedp.ByQuery),
			chromedp.Text(target, &text, chromedp.ByQuery),
		)
		if err := chromedp.Run(taskCtx, actions...); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("get_text from %s: %s", url, err)}, nil
		}
		// Collapse excessive whitespace to keep the output manageable.
		text = collapseWhitespace(text)
		return ToolResult{Content: text}, nil

	case "screenshot":
		var buf []byte
		actions = append(actions, chromedp.CaptureScreenshot(&buf))
		if err := chromedp.Run(taskCtx, actions...); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("screenshot of %s: %s", url, err)}, nil
		}

		// Save screenshot to workspace so the agent can reference it later.
		fname := fmt.Sprintf("screenshot_%d.png", time.Now().UnixMilli())
		dest := filepath.Join(t.workspace, fname)
		if err := os.MkdirAll(t.workspace, 0755); err == nil {
			_ = os.WriteFile(dest, buf, 0644)
		}
		return ToolResult{
			Content: fmt.Sprintf("screenshot saved: %s (%d bytes)", fname, len(buf)),
		}, nil

	case "click":
		if selector == "" {
			return ToolResult{IsError: true, Content: "selector is required for click"}, nil
		}
		var text string
		actions = append(actions,
			chromedp.WaitVisible(selector, chromedp.ByQuery),
			chromedp.Click(selector, chromedp.ByQuery),
			chromedp.Sleep(500*time.Millisecond),
			chromedp.Text("body", &text, chromedp.ByQuery),
		)
		if err := chromedp.Run(taskCtx, actions...); err != nil {
			return ToolResult{IsError: true, Content: fmt.Sprintf("click %s on %s: %s", selector, url, err)}, nil
		}
		return ToolResult{Content: fmt.Sprintf("clicked %s\npage text after click:\n%s", selector, collapseWhitespace(text))}, nil

	default:
		return ToolResult{IsError: true, Content: fmt.Sprintf("unknown operation %q", op)}, nil
	}
}

// collapseWhitespace reduces multiple blank lines and trims long runs of spaces.
func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			blank++
			if blank <= 1 {
				out = append(out, l)
			}
		} else {
			blank = 0
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
