package internal

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func HandleProxy(c *gin.Context) {
	// Handle /v1/models endpoint
	if c.Request.URL.Path == "/v1/models" {
		HandleModels(c)
		return
	}

	// Route based on HTTP method
	switch c.Request.Method {
	case http.MethodGet:
		handleGetRequest(c)
	case http.MethodPost:
		handlePostRequest(c)
	default:
		// For other methods (PUT, DELETE, PATCH), use POST handler logic
		handlePostRequest(c)
	}
}

// handleGetRequest proxies GET requests (typically for fetching data)
func handleGetRequest(c *gin.Context) {
	ctx, err := buildRequestContext(c)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// GET requests don't have a body to analyze, default to Anthropic
	targetURL, authHeader, err := getDefaultAnthropicTarget()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	provider := ProviderAnthropic
	if strings.Contains(targetURL.Host, "kimi.com") {
		provider = ProviderKimi
	}

	ctx.Target = &ProxyTarget{
		URL:        targetURL,
		AuthHeader: authHeader,
		Provider:   provider,
	}

	executeProxy(c, ctx)
}

// handlePostRequest proxies POST requests (typically for chat completions)
func handlePostRequest(c *gin.Context) {
	ctx, err := buildRequestContext(c)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Determine target based on model in body
	targetURL, authHeader, err := determineTarget(ctx.BodyBytes)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	provider := ProviderAnthropic
	if strings.Contains(targetURL.Host, "kimi.com") {
		provider = ProviderKimi
	}

	ctx.Target = &ProxyTarget{
		URL:        targetURL,
		AuthHeader: authHeader,
		Provider:   provider,
	}

	executeProxy(c, ctx)
}

// buildRequestContext extracts and preserves request context
func buildRequestContext(c *gin.Context) (*RequestContext, error) {
	// Read body for potential analysis
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read body: %w", err)
	}
	// Restore body for proxying
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Capture original headers before Gin modifies them
	originalHeaders := make(http.Header)
	for key, values := range c.Request.Header {
		if preserveHeaders[key] {
			originalHeaders[key] = values
		}
	}

	// Debug: log incoming request headers
	if os.Getenv("DEBUG") == "1" {
		fmt.Printf("  [DEBUG] Incoming headers:\n")
		for key, values := range c.Request.Header {
			for _, v := range values {
				fmt.Printf("  [DEBUG]   %s: %s\n", key, v)
			}
		}
	}

	return &RequestContext{
		OriginalHeaders: originalHeaders,
		BodyBytes:       bodyBytes,
	}, nil
}

// executeProxy performs the actual proxying
func executeProxy(c *gin.Context, ctx *RequestContext) {
	targetURL := ctx.Target.URL
	authHeader := ctx.Target.AuthHeader
	provider := ctx.Target.Provider
	originalHeaders := ctx.OriginalHeaders

	// Use httputil.ReverseProxy for proper HTTP proxying
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Customize the director to properly handle headers
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		// Clear Authorization from incoming request before director copies headers
		// This prevents the client's token from being forwarded
		c.Request.Header.Del("Authorization")
		originalDirector(req)

		// Set the correct Host header for the target
		req.Host = targetURL.Host

		// Restore original headers (whitelist approach - only preserve known headers)
		for key, values := range originalHeaders {
			// Don't overwrite if already set by originalDirector
			if _, exists := req.Header[key]; !exists && len(values) > 0 {
				req.Header[key] = values
			}
		}

		// Ensure content-type if not present
		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}

		// Set authorization based on token type
		// API keys (sk-ant-api03-) use x-api-key header
		// OAuth tokens (sk-ant-oat-) use Authorization: Bearer
		if strings.HasPrefix(authHeader, "Bearer sk-ant-api03-") {
			req.Header.Del("Authorization")
			req.Header.Set("x-api-key", strings.TrimPrefix(authHeader, "Bearer "))
		} else {
			req.Header.Set("Authorization", authHeader)
		}

		// Add provider-specific headers only if not already present
		switch provider {
		case ProviderAnthropic:
			// Always ensure oauth-2025-04-20 is in the beta list (required for OAuth tokens)
			currentBetas := req.Header.Get("anthropic-beta")
			if !strings.Contains(currentBetas, "oauth-2025-04-20") {
				if currentBetas == "" {
					req.Header.Set("anthropic-beta", AnthropicOAuthBeta)
				} else {
					req.Header.Set("anthropic-beta", currentBetas+","+AnthropicOAuthBeta)
				}
			}
			if req.Header.Get("anthropic-version") == "" {
				req.Header.Set("anthropic-version", AnthropicVersion)
			}
		case ProviderKimi:
			if req.Header.Get("User-Agent") == "" {
				req.Header.Set("User-Agent", KimiUserAgent)
			}
		}

		// Debug: log outgoing request headers
		if os.Getenv("DEBUG") == "1" {
			fmt.Printf("  [DEBUG] Outgoing headers:\n")
			for key, values := range req.Header {
				for _, v := range values {
					fmt.Printf("  [DEBUG]   %s: %s\n", key, v)
				}
			}
		}

		// Standard logging
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
