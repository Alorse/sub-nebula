package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"sub-nebula/internal"
)

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
	r.Any("/v1/*path", internal.HandleProxy)
	r.Any("/v1", internal.HandleProxy)

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	fmt.Printf("🚀 Sub-Nebula proxy started on http://localhost:%s\n", internal.Config.Port)
	fmt.Printf("📍 Anthropic API: %s\n", internal.Config.AnthropicBaseURL)
	fmt.Printf("📍 Kimi API: %s\n", internal.Config.KimiBaseURL)
	fmt.Printf("🤖 Subagent model: %s\n", internal.Config.SubagentModel)

	r.Run(":" + internal.Config.Port)
}

func loadConfig() {
	internal.Config.Port = getEnv("PORT", "4242")
	// Force correct Anthropic URL - ignore env var that may be contaminated
	internal.Config.AnthropicBaseURL = "https://api.anthropic.com"
	internal.Config.KimiBaseURL = getEnv("KIMI_BASE_URL", "https://api.kimi.com/coding")
	internal.Config.KimiAPIKey = getEnv("KIMI_API_KEY", "")
	internal.Config.SubagentModel = getEnv("SUBAGENT_MODEL", "kimi-for-coding")
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
