package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Proxy configuration
type Config struct {
	Port             string
	AnthropicBaseURL string
	KimiBaseURL      string
	KimiAPIKey       string
	SubagentModel    string
}

// Constants for API headers
const (
	AnthropicOAuthBeta    = "oauth-2025-04-20"
	AnthropicVersion      = "2023-06-01"
	KimiUserAgent         = "KimiCLI/1.19.0"
	AuthHeaderAnthropic   = "anthropic"
	AuthHeaderKimi        = "kimi"
)

// Claude Code credentials structure
type ClaudeCredentials struct {
	ClaudeAiOauth struct {
		AccessToken  string   `json:"accessToken"`
		RefreshToken string   `json:"refreshToken"`
		ExpiresAt    int64    `json:"expiresAt"`
		Scopes       []string `json:"scopes"`
		SubType      string   `json:"subscriptionType"`
		RateLimit    string   `json:"rateLimitTier"`
	} `json:"claudeAiOauth"`
}

// Token cache
type TokenCache struct {
	token     string
	expiresAt int64
	mu        sync.RWMutex
}

var (
	config     Config
	tokenCache TokenCache
)

func main() {
	// Load configuration
	loadConfig()

	// Setup Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger())

	// Proxy endpoint - intercepts all requests
	r.Any("/v1/*path", handleProxy)
	r.Any("/v1", handleProxy)

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	fmt.Printf("🚀 Sub-Nebula proxy started on http://localhost:%s\n", config.Port)
	fmt.Printf("📍 Anthropic API: %s\n", config.AnthropicBaseURL)
	fmt.Printf("📍 Kimi API: %s\n", config.KimiBaseURL)
	fmt.Printf("🤖 Subagent model: %s\n", config.SubagentModel)

	r.Run(":" + config.Port)
}

func loadConfig() {
	config.Port = getEnv("PORT", "4242")
	// Force correct Anthropic URL - ignore env var that may be contaminated
	config.AnthropicBaseURL = "https://api.anthropic.com"
	config.KimiBaseURL = getEnv("KIMI_BASE_URL", "https://api.kimi.com/coding")
	config.KimiAPIKey = getEnv("KIMI_API_KEY", "")
	config.SubagentModel = getEnv("SUBAGENT_MODEL", "kimi-for-coding")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		duration := time.Since(start)
		fmt.Printf("[%s] %s %s - %d (%v)\n",
			time.Now().Format("15:04:05"),
			c.Request.Method,
			path,
			c.Writer.Status(),
			duration,
		)
	}
}

func handleProxy(c *gin.Context) {
	// Read body to analyze model
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(400, gin.H{"error": "cannot read body"})
		return
	}
	// Restore body for potential re-reads
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Determine target model
	targetURL, authHeader, err := determineTarget(bodyBytes)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Use httputil.ReverseProxy for proper HTTP proxying
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Determine provider type once to avoid repeated string matching
	provider := AuthHeaderAnthropic
	if strings.Contains(targetURL.Host, "kimi.com") {
		provider = AuthHeaderKimi
	}

	// Customize the director to properly handle headers
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Set the correct Host header for the target
		req.Host = targetURL.Host

		// Ensure content-type
		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}

		// Set authorization (use original if provided, otherwise use determined)
		if req.Header.Get("Authorization") == "" {
			req.Header.Set("Authorization", authHeader)
		}

		// Add provider-specific headers
		if provider == AuthHeaderAnthropic {
			if req.Header.Get("anthropic-beta") == "" {
				req.Header.Set("anthropic-beta", AnthropicOAuthBeta)
			}
			if req.Header.Get("anthropic-version") == "" {
				req.Header.Set("anthropic-version", AnthropicVersion)
			}
		} else if provider == AuthHeaderKimi {
			req.Header.Set("User-Agent", KimiUserAgent)
		}

		// Debug logging
		fmt.Printf("  → Proxying to: %s%s\n", targetURL.Host, c.Request.URL.Path)
		auth := req.Header.Get("Authorization")
		if len(auth) > 30 {
			fmt.Printf("  → Auth header: %s...\n", auth[:30])
		} else {
			fmt.Printf("  → Auth header: %s\n", auth)
		}
		fmt.Printf("  → anthropic-beta: %s\n", req.Header.Get("anthropic-beta"))
		fmt.Printf("  → anthropic-version: %s\n", req.Header.Get("anthropic-version"))
		fmt.Printf("  → User-Agent: %s\n", req.Header.Get("User-Agent"))
		fmt.Printf("  → Content-Length: %d\n", req.ContentLength)
	}

	// Handle errors
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		fmt.Printf("  ✗ Proxy error: %v\n", err)
		c.JSON(502, gin.H{"error": "proxy error", "details": err.Error()})
	}

	// Execute the proxy
	proxy.ServeHTTP(c.Writer, c.Request)
}

func determineTarget(body []byte) (*url.URL, string, error) {
	// Extract model from body
	var request struct {
		Model string `json:"model"`
	}

	if err := json.Unmarshal(body, &request); err != nil {
		// If no model, default to Anthropic
		targetURL, _ := url.Parse(config.AnthropicBaseURL)
		token, err := getClaudeToken()
		if err != nil {
			return nil, "", err
		}
		return targetURL, "Bearer " + token, nil
	}

	model := request.Model
	fmt.Printf("  📦 Model requested: %s\n", model)

	// If it's the subagent model, go to Kimi
	if model == config.SubagentModel || strings.HasPrefix(model, "kimi-") {
		targetURL, err := url.Parse(config.KimiBaseURL)
		if err != nil {
			return nil, "", err
		}
		fmt.Printf("  🔄 Routing to: Kimi\n")
		return targetURL, "Bearer " + config.KimiAPIKey, nil
	}

	// Default: Anthropic (Claude)
	targetURL, err := url.Parse(config.AnthropicBaseURL)
	if err != nil {
		return nil, "", err
	}

	token, err := getClaudeToken()
	if err != nil {
		return nil, "", err
	}

	fmt.Printf("  🔄 Routing to: Anthropic\n")
	return targetURL, "Bearer " + token, nil
}

// getClaudeToken retrieves the OAuth token from Claude Code's system storage
func getClaudeToken() (string, error) {
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
