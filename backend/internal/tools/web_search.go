package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebSearchTool searches the web using Brave Search (primary) or SerpAPI (fallback).
type WebSearchTool struct {
	braveKey string
	serpKey  string
	client   *http.Client
}

func NewWebSearchTool(braveKey, serpKey string) *WebSearchTool {
	return &WebSearchTool{
		braveKey: braveKey,
		serpKey:  serpKey,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Description() string {
	return "Search the web and return relevant results (title, URL, snippet)."
}

func (t *WebSearchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
			"num_results": map[string]any{
				"type":        "integer",
				"description": "Number of results to return (default 5, max 10)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return ToolResult{IsError: true, Content: "query is required"}, nil
	}
	n := 5
	if v, ok := input["num_results"].(float64); ok && v > 0 {
		n = int(v)
		if n > 10 {
			n = 10
		}
	}

	if t.braveKey != "" {
		if result, err := t.brave(ctx, query, n); err == nil {
			return ToolResult{Content: result}, nil
		}
	}
	if t.serpKey != "" {
		if result, err := t.serp(ctx, query, n); err == nil {
			return ToolResult{Content: result}, nil
		}
	}
	return ToolResult{IsError: true, Content: "no search API configured (set BRAVE_API_KEY or SERP_API_KEY)"}, nil
}

func (t *WebSearchTool) brave(ctx context.Context, query string, n int) (string, error) {
	u := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), n)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", t.braveKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("brave: HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	var data struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				URL         string `json:"url"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}

	var sb strings.Builder
	for i, r := range data.Web.Results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description)
	}
	return sb.String(), nil
}

func (t *WebSearchTool) serp(ctx context.Context, query string, n int) (string, error) {
	u := fmt.Sprintf("https://serpapi.com/search?q=%s&api_key=%s&num=%d",
		url.QueryEscape(query), t.serpKey, n)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("serpapi: HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	var data struct {
		OrganicResults []struct {
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
			Link    string `json:"link"`
		} `json:"organic_results"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}

	var sb strings.Builder
	for i, r := range data.OrganicResults {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.Link, r.Snippet)
	}
	return sb.String(), nil
}
