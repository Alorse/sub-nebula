package internal

import (
	"net/http"
	"net/url"
	"sync"
	"time"
)

// HTTPClient is the shared HTTP client for all requests
var HTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

// Config holds the application configuration
var Config = struct {
	Port             string
	AnthropicBaseURL string
	KimiBaseURL      string
	KimiAPIKey       string
	SubagentModel    string
}{
	Port:             "4242",
	AnthropicBaseURL: "https://api.anthropic.com",
	KimiBaseURL:      "https://api.kimi.com/coding",
	SubagentModel:    "kimi-for-coding",
}

// Constants for API headers
const (
	AnthropicOAuthBeta = "oauth-2025-04-20"
	AnthropicVersion   = "2023-06-01"
	KimiUserAgent      = "KimiCLI/1.19.0"
)

// ProviderType represents the type of API provider
type ProviderType string

const (
	ProviderAnthropic ProviderType = "anthropic"
	ProviderKimi      ProviderType = "kimi"
)

// Model represents an API model
type Model struct {
	Created int64  `json:"created"`
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// ProviderModel represents a model from a provider's API (format varies)
// Different providers use different field names for timestamps
type ProviderModel struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"` // ISO 8601 format (Anthropic)
	Created   int64  `json:"created"`    // Unix timestamp (Kimi)
}

// ModelsResponse represents the /v1/models response
type ModelsResponse struct {
	Data   []Model `json:"data"`
	Object string  `json:"object"`
}

// ProviderModelsResponse represents a provider's response format
type ProviderModelsResponse struct {
	Data   []ProviderModel `json:"data"`
	Object string          `json:"object"`
}

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

// ProxyTarget holds the resolved target for a request
type ProxyTarget struct {
	URL        *url.URL
	AuthHeader string
	Provider   ProviderType
}

// RequestContext holds all context needed for proxying a request
type RequestContext struct {
	OriginalHeaders http.Header
	BodyBytes       []byte
	Target          *ProxyTarget
}

// providerFetchConfig holds configuration for fetching from a provider
type providerFetchConfig struct {
	name      string
	baseURL   string
	authFunc  func() (string, error)
	userAgent string
	provider  ProviderType
}

// Headers to preserve from original request (whitelist approach)
var preserveHeaders = map[string]bool{
	"Authorization":     true,
	"Content-Type":      true,
	"Content-Length":    true,
	"Accept":            true,
	"Accept-Encoding":   true,
	"Accept-Language":   true,
	"Cache-Control":     true,
	"Connection":        true,
	"User-Agent":        true,
	"X-Request-ID":      true,
	"X-Correlation-ID":  true,
	"anthropic-beta":    true,
	"anthropic-version": true,
	"anthropic-client":  true,
	"x-api-key":         true,
}
