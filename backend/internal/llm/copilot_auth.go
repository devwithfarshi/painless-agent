package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// package-level vars so tests can inject mock servers and suppress output
var (
	copilotDeviceCodeEndpoint    = "https://github.com/login/device/code"
	copilotOAuthEndpoint         = "https://github.com/login/oauth/access_token"
	copilotInternalTokenEndpoint = "https://api.github.com/copilot_internal/v2/token"
	copilotHTTPClient            = http.DefaultClient
	// copilotAuthHTTPClient is used exclusively for short auth/token-exchange requests.
	// A hard 30-second timeout prevents the polling loop from hanging indefinitely when
	// the network stalls after the user has already authorized on GitHub.
	copilotAuthHTTPClient = &http.Client{Timeout: 30 * time.Second}
	// deviceFlowWriter is where authorization UX messages are printed.
	// Tests set this to io.Discard to suppress output.
	deviceFlowWriter io.Writer = os.Stdout
)

var acceptedGHTokenPrefixes = []string{"gho_", "github_pat_", "ghu_"}

// validateGitHubToken returns an error for classic ghp_ PATs and unknown prefixes.
func validateGitHubToken(token string) error {
	if strings.HasPrefix(token, "ghp_") {
		return fmt.Errorf("classic GitHub PATs (ghp_) are not accepted by the Copilot API; use device-code login or an OAuth/fine-grained token (gho_, github_pat_, ghu_)")
	}
	for _, pfx := range acceptedGHTokenPrefixes {
		if strings.HasPrefix(token, pfx) {
			return nil
		}
	}
	return fmt.Errorf("unsupported GitHub token prefix; accepted: gho_, github_pat_, ghu_")
}

// ResolveGitHubToken returns a validated GitHub token using the following priority:
//  1. COPILOT_GITHUB_TOKEN env
//  2. GH_TOKEN env
//  3. GITHUB_TOKEN env
//  4. `gh auth token` CLI output
//  5. stored device-login token
//
// If no token is found or all fail validation, the device-code OAuth flow is started.
func ResolveGitHubToken(ctx context.Context) (string, error) {
	for _, key := range []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if t := os.Getenv(key); t != "" {
			if err := validateGitHubToken(t); err != nil {
				return "", fmt.Errorf("%s: %w", key, err)
			}
			return t, nil
		}
	}

	if t, err := ghCLIToken(ctx); err == nil && t != "" {
		if validateGitHubToken(t) == nil {
			return t, nil
		}
	}

	if t, err := loadStoredToken(); err == nil && t != "" {
		if validateGitHubToken(t) == nil {
			return t, nil
		}
	}

	return runDeviceCodeFlow(ctx)
}

func ghCLIToken(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// copilotTokenFilePath returns ~/.painless-agent/copilot_token.
func copilotTokenFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".painless-agent", "copilot_token"), nil
}

// loadStoredToken loads from OS keychain, then falls back to the token file.
func loadStoredToken() (string, error) {
	if t, err := keychainLoad(); err == nil && t != "" {
		return t, nil
	}
	path, err := copilotTokenFilePath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// storeGitHubToken saves the token to OS keychain, falling back to a 0600 file.
func storeGitHubToken(token string) error {
	if err := keychainSave(token); err == nil {
		return nil
	}
	return storeTokenFile(token)
}

// storeTokenFile writes the token to ~/.painless-agent/copilot_token with mode 0600.
func storeTokenFile(token string) error {
	path, err := copilotTokenFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create token dir: %w", err)
	}
	return os.WriteFile(path, []byte(token), 0600)
}

// --- Device-code OAuth flow ---

type deviceCodeResp struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type oauthPollResp struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	// Interval is set by GitHub on "slow_down" responses to tell us the new poll interval.
	Interval int `json:"interval"`
}

func runDeviceCodeFlow(ctx context.Context) (string, error) {
	clientID := os.Getenv("COPILOT_GITHUB_CLIENT_ID")
	if clientID == "" {
		clientID = "Iv1.b507a08c87ecfe98"
	}

	dc, err := requestDeviceCode(ctx, clientID)
	if err != nil {
		return "", fmt.Errorf("device code: %w", err)
	}

	interval := dc.Interval
	if interval <= 0 {
		interval = 5
	}

	fmt.Fprintf(deviceFlowWriter, "\nGitHub Copilot authorization required.\n")
	fmt.Fprintf(deviceFlowWriter, "  Visit: %s\n", dc.VerificationURI)
	fmt.Fprintf(deviceFlowWriter, "  Code:  %s\n", dc.UserCode)
	fmt.Fprintf(deviceFlowWriter, "  Code expires in %d seconds. Waiting for authorization...\n\n", dc.ExpiresIn)

	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	// doPoll makes one attempt and returns (token, done, err).
	// On "authorization_pending" it prints a dot and returns ("", false, nil).
	// On "slow_down" it sets interval to GitHub's prescribed value and returns ("", false, nil).
	doPoll := func() (string, bool, error) {
		if time.Now().After(deadline) {
			return "", false, fmt.Errorf("device code expired; run again to restart authorization")
		}
		tok, pollErr := pollOAuthToken(ctx, clientID, dc.DeviceCode)
		if pollErr != nil {
			return "", false, fmt.Errorf("poll token: %w", pollErr)
		}
		if tok.Error == "" {
			if tok.AccessToken == "" {
				return "", false, fmt.Errorf("empty access_token in poll response")
			}
			fmt.Fprintf(deviceFlowWriter, "\n✓ Authorization received! Continuing...\n")
			if saveErr := storeGitHubToken(tok.AccessToken); saveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to store GitHub token: %v\n", saveErr)
			}
			return tok.AccessToken, true, nil
		}
		switch tok.Error {
		case "authorization_pending":
			fmt.Fprintf(deviceFlowWriter, ".")
			return "", false, nil
		case "slow_down":
			// GitHub prescribes the new minimum interval; honour it exactly.
			if tok.Interval > interval {
				interval = tok.Interval
			} else {
				interval += 5 // RFC 8628 fallback: add 5 seconds if no hint
			}
			return "", false, nil
		case "expired_token":
			return "", false, fmt.Errorf("device code expired; run again to restart authorization")
		case "access_denied":
			return "", false, fmt.Errorf("authorization denied; run again to restart")
		default:
			return "", false, fmt.Errorf("poll token: unexpected error %q", tok.Error)
		}
	}

	// Poll once immediately so a user who authorized right away gets a near-instant response
	// instead of waiting a full interval (up to 5 s) for the first ticker tick.
	if token, done, err := doPoll(); err != nil || done {
		return token, err
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	prevInterval := interval
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			token, done, err := doPoll()
			if err != nil {
				return "", err
			}
			if done {
				return token, nil
			}
			if interval != prevInterval {
				ticker.Reset(time.Duration(interval) * time.Second)
				prevInterval = interval
			}
		}
	}
}

func requestDeviceCode(ctx context.Context, clientID string) (*deviceCodeResp, error) {
	body := "client_id=" + clientID + "&scope=read:user"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, copilotDeviceCodeEndpoint,
		strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := copilotAuthHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var dc deviceCodeResp
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
		return nil, fmt.Errorf("decode device code response: %w", err)
	}
	if dc.DeviceCode == "" {
		return nil, fmt.Errorf("empty device_code in response")
	}
	return &dc, nil
}

// pollOAuthToken polls the OAuth token endpoint.
// On success, returns the poll response with AccessToken set.
// On transient states ("authorization_pending", "slow_down"), returns the response
// (with Error set) so callers can read the Interval hint from slow_down responses.
// On terminal errors ("expired_token", "access_denied") returns an error.
func pollOAuthToken(ctx context.Context, clientID, deviceCode string) (*oauthPollResp, error) {
	body := "client_id=" + clientID +
		"&device_code=" + deviceCode +
		"&grant_type=urn:ietf:params:oauth:grant-type:device_code"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, copilotOAuthEndpoint,
		strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := copilotAuthHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tok oauthPollResp
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decode poll response: %w", err)
	}
	return &tok, nil
}

// --- Copilot API token exchange ---

type copilotAPIToken struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"` // Unix timestamp seconds
}

// ExchangeCopilotToken exchanges a GitHub token for a short-lived Copilot API token.
func ExchangeCopilotToken(ctx context.Context, githubToken string) (*copilotAPIToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, copilotInternalTokenEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "painless-agent/0.1.0")

	resp, err := copilotAuthHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot token exchange returned HTTP %d", resp.StatusCode)
	}

	var ct copilotAPIToken
	if err := json.NewDecoder(resp.Body).Decode(&ct); err != nil {
		return nil, fmt.Errorf("decode copilot token response: %w", err)
	}
	if ct.Token == "" {
		return nil, fmt.Errorf("empty token in copilot token response")
	}
	return &ct, nil
}

// FetchCopilotModels returns the list of model IDs available via the Copilot API.
// githubToken is the OAuth token returned by ResolveGitHubToken.
func FetchCopilotModels(ctx context.Context, githubToken string) ([]string, error) {
	ct, err := ExchangeCopilotToken(ctx, githubToken)
	if err != nil {
		return nil, fmt.Errorf("exchange token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.githubcopilot.com/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+ct.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Editor-Version", "painless-agent/0.1.0")
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("Openai-Intent", "conversation-panel")
	req.Header.Set("x-initiator", "user")

	resp, err := copilotHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	models := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		if m.ID != "" && isCopilotChatModel(m.ID) {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

// isCopilotChatModel filters out models that are not accessible via /chat/completions:
// embeddings, internal routing models, code-completion (codex) models, and editor-specific stubs.
func isCopilotChatModel(id string) bool {
	excludePrefixes := []string{
		"text-embedding",
		"mai-code",
		"trajectory",
		"oswe-",
	}
	excludeSuffixes := []string{
		"-codex",
		"-picker",
		"-secondary",
		"-tertiary",
	}
	for _, p := range excludePrefixes {
		if len(id) >= len(p) && id[:len(p)] == p {
			return false
		}
	}
	for _, s := range excludeSuffixes {
		if len(id) >= len(s) && id[len(id)-len(s):] == s {
			return false
		}
	}
	return true
}
