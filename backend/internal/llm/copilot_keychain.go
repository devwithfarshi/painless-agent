package llm

import "fmt"

// keychainSave saves token to the OS keychain.
// This stub always returns an error, causing storeGitHubToken to fall back to
// ~/.painless-agent/copilot_token (mode 0600). Add a platform-specific
// implementation (e.g., via github.com/zalando/go-keyring) to enable keychain support.
func keychainSave(_ string) error { return fmt.Errorf("OS keychain not configured") }

// keychainLoad loads the token from the OS keychain.
// This stub always returns an error; the caller falls back to the token file.
func keychainLoad() (string, error) { return "", fmt.Errorf("OS keychain not configured") }
