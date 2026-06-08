package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	// Suppress device-flow UX output in all tests.
	deviceFlowWriter = io.Discard
}

// --- token validation ---

func TestValidateGitHubToken_Accepted(t *testing.T) {
	cases := []string{
		"gho_valid123",
		"github_pat_fine_grained",
		"ghu_usertoken",
	}
	for _, tok := range cases {
		if err := validateGitHubToken(tok); err != nil {
			t.Errorf("expected %q to be valid, got: %v", tok, err)
		}
	}
}

func TestValidateGitHubToken_RejectsGhp(t *testing.T) {
	err := validateGitHubToken("ghp_classicPAT1234567890")
	if err == nil {
		t.Fatal("expected ghp_ token to be rejected")
	}
	if !strings.Contains(err.Error(), "ghp_") {
		t.Errorf("error should mention ghp_, got: %v", err)
	}
}

func TestValidateGitHubToken_RejectsUnknownPrefix(t *testing.T) {
	if err := validateGitHubToken("unknown_prefix_token"); err == nil {
		t.Fatal("expected unknown prefix to be rejected")
	}
}

// --- token resolution priority ---

func TestTokenResolutionPriority_CopilotFirst(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_copilot")
	t.Setenv("GH_TOKEN", "gho_gh")
	t.Setenv("GITHUB_TOKEN", "gho_github")

	// All three are valid gho_ tokens; resolveGitHubToken must return COPILOT_GITHUB_TOKEN.
	ctx := context.Background()
	tok, err := ResolveGitHubToken(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "gho_copilot" {
		t.Errorf("expected gho_copilot (COPILOT_GITHUB_TOKEN), got %q", tok)
	}
}

func TestTokenResolutionPriority_GHTokenFallback(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "gho_gh_fallback")
	t.Setenv("GITHUB_TOKEN", "gho_github_token")

	ctx := context.Background()
	tok, err := ResolveGitHubToken(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "gho_gh_fallback" {
		t.Errorf("expected gho_gh_fallback, got %q", tok)
	}
}

func TestTokenResolutionPriority_GitHubTokenFallback(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "gho_github_only")

	ctx := context.Background()
	tok, err := ResolveGitHubToken(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "gho_github_only" {
		t.Errorf("expected gho_github_only, got %q", tok)
	}
}

func TestTokenResolutionPriority_RejectsGhpFromEnv(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "ghp_classicPAT")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	ctx := context.Background()
	_, err := ResolveGitHubToken(ctx)
	if err == nil {
		t.Fatal("expected error for ghp_ token in COPILOT_GITHUB_TOKEN")
	}
	if !strings.Contains(err.Error(), "ghp_") {
		t.Errorf("error should mention ghp_, got: %v", err)
	}
}

// --- device-code polling states ---

func TestDeviceCodePolling_EventualSuccess(t *testing.T) {
	var pollCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			json.NewEncoder(w).Encode(deviceCodeResp{
				DeviceCode:      "dev_code_abc",
				UserCode:        "ABCD-1234",
				VerificationURI: "https://github.com/login/device",
				ExpiresIn:       900,
				Interval:        1, // 1-second poll interval for fast tests
			})
		case "/login/oauth/access_token":
			n := atomic.AddInt32(&pollCount, 1)
			if n < 3 {
				json.NewEncoder(w).Encode(oauthPollResp{Error: "authorization_pending"})
				return
			}
			json.NewEncoder(w).Encode(oauthPollResp{AccessToken: "gho_authorized"})
		}
	}))
	defer srv.Close()

	origDC, origOA, origAuthClient := copilotDeviceCodeEndpoint, copilotOAuthEndpoint, copilotAuthHTTPClient
	copilotDeviceCodeEndpoint = srv.URL + "/login/device/code"
	copilotOAuthEndpoint = srv.URL + "/login/oauth/access_token"
	copilotAuthHTTPClient = srv.Client()
	defer func() {
		copilotDeviceCodeEndpoint = origDC
		copilotOAuthEndpoint = origOA
		copilotAuthHTTPClient = origAuthClient
	}()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("COPILOT_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	// Immediate poll (count=1, pending) + 2 ticker polls → success on poll 3.
	token, err := runDeviceCodeFlow(context.Background())
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if token != "gho_authorized" {
		t.Errorf("expected gho_authorized, got %q", token)
	}
	if atomic.LoadInt32(&pollCount) < 3 {
		t.Errorf("expected at least 3 polls, got %d", atomic.LoadInt32(&pollCount))
	}
}

func TestDeviceCodePolling_AccessDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			json.NewEncoder(w).Encode(deviceCodeResp{
				DeviceCode: "dev_code_deny", UserCode: "DENY-0000",
				VerificationURI: "https://github.com/login/device", ExpiresIn: 900, Interval: 1,
			})
		case "/login/oauth/access_token":
			json.NewEncoder(w).Encode(oauthPollResp{Error: "access_denied"})
		}
	}))
	defer srv.Close()

	origDC, origOA, origAuthClient := copilotDeviceCodeEndpoint, copilotOAuthEndpoint, copilotAuthHTTPClient
	copilotDeviceCodeEndpoint = srv.URL + "/login/device/code"
	copilotOAuthEndpoint = srv.URL + "/login/oauth/access_token"
	copilotAuthHTTPClient = srv.Client()
	defer func() {
		copilotDeviceCodeEndpoint = origDC
		copilotOAuthEndpoint = origOA
		copilotAuthHTTPClient = origAuthClient
	}()

	t.Setenv("HOME", t.TempDir())

	_, err := runDeviceCodeFlow(context.Background())
	if err == nil {
		t.Fatal("expected error for access_denied")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Errorf("expected 'denied' in error, got: %v", err)
	}
}

func TestDeviceCodePolling_ExpiredToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			json.NewEncoder(w).Encode(deviceCodeResp{
				DeviceCode: "dev_code_exp", UserCode: "EXPI-1234",
				VerificationURI: "https://github.com/login/device", ExpiresIn: 900, Interval: 1,
			})
		case "/login/oauth/access_token":
			json.NewEncoder(w).Encode(oauthPollResp{Error: "expired_token"})
		}
	}))
	defer srv.Close()

	origDC, origOA, origAuthClient := copilotDeviceCodeEndpoint, copilotOAuthEndpoint, copilotAuthHTTPClient
	copilotDeviceCodeEndpoint = srv.URL + "/login/device/code"
	copilotOAuthEndpoint = srv.URL + "/login/oauth/access_token"
	copilotAuthHTTPClient = srv.Client()
	defer func() {
		copilotDeviceCodeEndpoint = origDC
		copilotOAuthEndpoint = origOA
		copilotAuthHTTPClient = origAuthClient
	}()

	t.Setenv("HOME", t.TempDir())

	_, err := runDeviceCodeFlow(context.Background())
	if err == nil {
		t.Fatal("expected error for expired_token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected 'expired' in error, got: %v", err)
	}
}

func TestDeviceCodePolling_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			json.NewEncoder(w).Encode(deviceCodeResp{
				DeviceCode: "dev_code_cancel", UserCode: "CNCL-1234",
				VerificationURI: "https://github.com/login/device", ExpiresIn: 900, Interval: 1,
			})
		case "/login/oauth/access_token":
			json.NewEncoder(w).Encode(oauthPollResp{Error: "authorization_pending"})
		}
	}))
	defer srv.Close()

	origDC, origOA, origAuthClient := copilotDeviceCodeEndpoint, copilotOAuthEndpoint, copilotAuthHTTPClient
	copilotDeviceCodeEndpoint = srv.URL + "/login/device/code"
	copilotOAuthEndpoint = srv.URL + "/login/oauth/access_token"
	copilotAuthHTTPClient = srv.Client()
	defer func() {
		copilotDeviceCodeEndpoint = origDC
		copilotOAuthEndpoint = origOA
		copilotAuthHTTPClient = origAuthClient
	}()

	t.Setenv("HOME", t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := runDeviceCodeFlow(ctx)
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

// --- local token storage permissions ---

func TestLocalTokenStoragePermissions(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	token := "gho_storage_test_token"
	if err := storeTokenFile(token); err != nil {
		t.Fatalf("storeTokenFile: %v", err)
	}

	path := filepath.Join(tmpHome, ".painless-agent", "copilot_token")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}

	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("expected mode 0600, got %04o", mode)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if string(data) != token {
		t.Errorf("expected %q, got %q", token, string(data))
	}
}

func TestLocalTokenStorageRoundtrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	token := "gho_roundtrip_token"
	if err := storeTokenFile(token); err != nil {
		t.Fatalf("storeTokenFile: %v", err)
	}

	loaded, err := loadStoredToken()
	if err != nil {
		t.Fatalf("loadStoredToken: %v", err)
	}
	if loaded != token {
		t.Errorf("expected %q, got %q", token, loaded)
	}
}

// --- Copilot token exchange parsing ---

func TestCopilotTokenExchangeParsing(t *testing.T) {
	expiresAt := time.Now().Add(30 * time.Minute).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "token github_pat_test123" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(copilotAPIToken{
			Token:     "copilot_api_xyz",
			ExpiresAt: expiresAt,
		})
	}))
	defer srv.Close()

	origURL, origAuthClient := copilotInternalTokenEndpoint, copilotAuthHTTPClient
	copilotInternalTokenEndpoint = srv.URL
	copilotAuthHTTPClient = srv.Client()
	defer func() {
		copilotInternalTokenEndpoint = origURL
		copilotAuthHTTPClient = origAuthClient
	}()

	ct, err := ExchangeCopilotToken(context.Background(), "github_pat_test123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct.Token != "copilot_api_xyz" {
		t.Errorf("expected copilot_api_xyz, got %q", ct.Token)
	}
	if ct.ExpiresAt != expiresAt {
		t.Errorf("expected expiresAt %d, got %d", expiresAt, ct.ExpiresAt)
	}
}

func TestCopilotTokenExchangeHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	origURL, origAuthClient := copilotInternalTokenEndpoint, copilotAuthHTTPClient
	copilotInternalTokenEndpoint = srv.URL
	copilotAuthHTTPClient = srv.Client()
	defer func() {
		copilotInternalTokenEndpoint = origURL
		copilotAuthHTTPClient = origAuthClient
	}()

	_, err := ExchangeCopilotToken(context.Background(), "gho_some_token")
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected 403 in error, got: %v", err)
	}
}

// --- 401 refresh retry ---

func Test401RefreshRetry_Complete(t *testing.T) {
	var chatCalls int32
	var tokenExchanges int32

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&tokenExchanges, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(copilotAPIToken{
			Token:     fmt.Sprintf("copilot_tok_%d", n),
			ExpiresAt: time.Now().Add(30 * time.Minute).Unix(),
		})
	}))
	defer tokenSrv.Close()

	chatSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&chatCalls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatCompletionResponse{
			Choices: []chatChoice{{Message: chatMessage{Content: "pong"}}},
			Usage:   chatUsage{PromptTokens: 5, CompletionTokens: 1},
		})
	}))
	defer chatSrv.Close()

	origToken, origChat, origAuthClient := copilotInternalTokenEndpoint, copilotChatEndpoint, copilotAuthHTTPClient
	copilotInternalTokenEndpoint = tokenSrv.URL
	copilotChatEndpoint = chatSrv.URL
	copilotAuthHTTPClient = tokenSrv.Client()
	defer func() {
		copilotInternalTokenEndpoint = origToken
		copilotChatEndpoint = origChat
		copilotAuthHTTPClient = origAuthClient
	}()

	p := NewCopilot("github_pat_test", "gpt-4o")
	resp, err := p.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("expected success after 401 retry, got: %v", err)
	}
	if resp.Content != "pong" {
		t.Errorf("expected 'pong', got %q", resp.Content)
	}
	if atomic.LoadInt32(&chatCalls) != 2 {
		t.Errorf("expected 2 chat calls, got %d", atomic.LoadInt32(&chatCalls))
	}
	if atomic.LoadInt32(&tokenExchanges) != 2 {
		t.Errorf("expected 2 token exchanges (initial + refresh), got %d", atomic.LoadInt32(&tokenExchanges))
	}
}

func Test401RefreshRetry_Stream(t *testing.T) {
	var chatCalls int32
	var tokenExchanges int32

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenExchanges, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(copilotAPIToken{
			Token:     "copilot_stream_tok",
			ExpiresAt: time.Now().Add(30 * time.Minute).Unix(),
		})
	}))
	defer tokenSrv.Close()

	chatSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&chatCalls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(streamChunk{
			Choices: []chatChoice{{Delta: chatMessage{Content: "hi"}}},
		}))
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer chatSrv.Close()

	origToken, origChat, origAuthClient := copilotInternalTokenEndpoint, copilotChatEndpoint, copilotAuthHTTPClient
	copilotInternalTokenEndpoint = tokenSrv.URL
	copilotChatEndpoint = chatSrv.URL
	copilotAuthHTTPClient = tokenSrv.Client()
	defer func() {
		copilotInternalTokenEndpoint = origToken
		copilotChatEndpoint = origChat
		copilotAuthHTTPClient = origAuthClient
	}()

	p := NewCopilot("github_pat_stream_test", "gpt-4o")
	ch, err := p.Stream(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error from Stream: %v", err)
	}

	var got strings.Builder
	for d := range ch {
		if d.Err != nil {
			t.Fatalf("stream error: %v", d.Err)
		}
		got.WriteString(d.Content)
	}
	if got.String() != "hi" {
		t.Errorf("expected 'hi', got %q", got.String())
	}
	if atomic.LoadInt32(&chatCalls) != 2 {
		t.Errorf("expected 2 chat calls, got %d", atomic.LoadInt32(&chatCalls))
	}
}

// --- token caching ---

func TestCopilotTokenCached(t *testing.T) {
	var exchanges int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&exchanges, 1)
		json.NewEncoder(w).Encode(copilotAPIToken{
			Token:     "cached_tok",
			ExpiresAt: time.Now().Add(30 * time.Minute).Unix(),
		})
	}))
	defer tokenSrv.Close()

	chatSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatCompletionResponse{
			Choices: []chatChoice{{Message: chatMessage{Content: "ok"}}},
		})
	}))
	defer chatSrv.Close()

	origToken, origChat, origAuthClient := copilotInternalTokenEndpoint, copilotChatEndpoint, copilotAuthHTTPClient
	copilotInternalTokenEndpoint = tokenSrv.URL
	copilotChatEndpoint = chatSrv.URL
	copilotAuthHTTPClient = tokenSrv.Client()
	defer func() {
		copilotInternalTokenEndpoint = origToken
		copilotChatEndpoint = origChat
		copilotAuthHTTPClient = origAuthClient
	}()

	p := NewCopilot("github_pat_cache", "gpt-4o")
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := p.Complete(ctx, CompletionRequest{
			Messages: []Message{{Role: RoleUser, Content: "ping"}},
		}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if n := atomic.LoadInt32(&exchanges); n != 1 {
		t.Errorf("expected 1 token exchange (cached), got %d", n)
	}
}

// --- Embed returns an error ---

func TestCopilotEmbed_ReturnsError(t *testing.T) {
	p := NewCopilot("gho_fake", "gpt-4o")
	_, err := p.Embed(context.Background(), "text")
	if err == nil {
		t.Fatal("expected Embed to return an error")
	}
}

// helpers

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
