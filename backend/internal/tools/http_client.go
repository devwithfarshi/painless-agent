package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPClientTool makes HTTP GET/POST requests with size and timeout limits.
type HTTPClientTool struct {
	client     *http.Client
	maxBodyB   int // max response body bytes
}

func NewHTTPClientTool(timeoutSecs, maxBodyKB int) *HTTPClientTool {
	return &HTTPClientTool{
		client:   &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second},
		maxBodyB: maxBodyKB * 1024,
	}
}

func (t *HTTPClientTool) Name() string { return "http_client" }

func (t *HTTPClientTool) Description() string {
	return "Make an HTTP GET or POST request to a URL and return the response body."
}

func (t *HTTPClientTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"method": map[string]any{
				"type":        "string",
				"enum":        []string{"GET", "POST"},
				"description": "HTTP method",
			},
			"url": map[string]any{
				"type":        "string",
				"description": "Request URL",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Request body (POST only)",
			},
			"headers": map[string]any{
				"type":                 "object",
				"description":          "Optional request headers",
				"additionalProperties": map[string]any{"type": "string"},
			},
		},
		"required": []string{"method", "url"},
	}
}

func (t *HTTPClientTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	method, _ := input["method"].(string)
	url, _ := input["url"].(string)
	body, _ := input["body"].(string)

	if url == "" {
		return ToolResult{IsError: true, Content: "url is required"}, nil
	}
	if method != "GET" && method != "POST" {
		method = "GET"
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("build request: %s", err)}, nil
	}

	if headers, ok := input["headers"].(map[string]any); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("request failed: %s", err)}, nil
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, int64(t.maxBodyB))
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("read body: %s", err)}, nil
	}

	return ToolResult{
		Content: fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, string(respBody)),
	}, nil
}
