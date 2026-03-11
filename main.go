package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// Shared transport for connection reuse (singleton pattern - prevents OOM from connection pool)
var sharedTransport = &http.Transport{
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 100,
	IdleConnTimeout:     90 * time.Second,
	ForceAttemptHTTP2:   true, // Ensure HTTP/2 support for Anthropic
}

// Shared HTTP client
var httpClient = &http.Client{
	Transport: sharedTransport,
	Timeout:   10 * time.Second,
}

var config Config

func main() {
	// Load .env file if it exists
	_ = godotenv.Load()

	// Load configuration
	loadConfig()

	// Setup Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger())

	// Proxy endpoint - intercepts all requests (including /v1/models)
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
