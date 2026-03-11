package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

var tokenCache TokenCache

// GetClaudeToken retrieves the OAuth token from Claude Code's system storage
func GetClaudeToken() (string, error) {
	tokenCache.mu.RLock()
	cachedToken := tokenCache.token
	cachedExpires := tokenCache.expiresAt
	tokenCache.mu.RUnlock()

	now := time.Now().Unix()

	// Use cache if still valid (with 5 minute buffer)
	if cachedToken != "" && cachedExpires > now+300 {
		fmt.Printf("  💾 Using cached token (expires in %d min)\n", (cachedExpires-now)/60)
		return cachedToken, nil
	}

	var creds ClaudeCredentials
	var err error

	if runtime.GOOS == "darwin" {
		creds, err = readMacOSCredentials()
	} else {
		creds, err = readFileCredentials()
	}

	if err != nil {
		return "", fmt.Errorf("failed to read Claude credentials: %w", err)
	}

	token := creds.ClaudeAiOauth.AccessToken
	if token == "" {
		return "", fmt.Errorf("no access token found in credentials")
	}

	// Update cache
	tokenCache.mu.Lock()
	tokenCache.token = token
	tokenCache.expiresAt = creds.ClaudeAiOauth.ExpiresAt / 1000 // Convert from ms to seconds
	tokenCache.mu.Unlock()

	fmt.Printf("  🔑 Token refreshed (expires: %s)\n",
		time.Unix(creds.ClaudeAiOauth.ExpiresAt/1000, 0).Format("15:04:05"))

	return token, nil
}

func readMacOSCredentials() (ClaudeCredentials, error) {
	var creds ClaudeCredentials

	cmd := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
	output, err := cmd.Output()
	if err != nil {
		return creds, fmt.Errorf("keychain access failed: %w", err)
	}

	if err := json.Unmarshal(output, &creds); err != nil {
		return creds, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return creds, nil
}

func readFileCredentials() (ClaudeCredentials, error) {
	var creds ClaudeCredentials

	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return creds, err
		}
		configDir = filepath.Join(home, ".claude")
	}

	credFile := filepath.Join(configDir, ".credentials.json")
	data, err := os.ReadFile(credFile)
	if err != nil {
		return creds, fmt.Errorf("cannot read credentials file: %w", err)
	}

	if err := json.Unmarshal(data, &creds); err != nil {
		return creds, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return creds, nil
}
