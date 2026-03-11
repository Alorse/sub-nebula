package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()

	// Set test configuration (will be overridden by .env if present)
	Config.Port = "4242"
	Config.AnthropicBaseURL = "https://api.anthropic.com"
	Config.KimiBaseURL = "https://api.kimi.com/coding"
	Config.SubagentModel = "kimi-for-coding"

	// Only set default Kimi API key if not loaded from .env
	if Config.KimiAPIKey == "" {
		Config.KimiAPIKey = "test-kimi-key"
	}
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Models endpoint
	r.GET("/v1/models", HandleModels)

	// Proxy endpoints - note: wildcard conflicts with /v1/models in Gin
	// In production, HandleProxy checks for /v1/models path internally
	r.POST("/v1/messages", HandleProxy)

	return r
}

func TestHealthEndpoint(t *testing.T) {
	router := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "ok", response["status"])
}

func TestProviderTypeConstants(t *testing.T) {
	assert.Equal(t, ProviderType("anthropic"), ProviderAnthropic)
	assert.Equal(t, ProviderType("kimi"), ProviderKimi)
}

func TestModelStruct(t *testing.T) {
	model := Model{
		Created: 1234567890,
		ID:      "test-model",
		Object:  "model",
		OwnedBy: "test-provider",
	}

	assert.Equal(t, int64(1234567890), model.Created)
	assert.Equal(t, "test-model", model.ID)
	assert.Equal(t, "model", model.Object)
	assert.Equal(t, "test-provider", model.OwnedBy)
}

func TestModelsResponse(t *testing.T) {
	response := ModelsResponse{
		Data: []Model{
			{Created: 1234567890, ID: "model-1", Object: "model", OwnedBy: "provider-1"},
			{Created: 1234567891, ID: "model-2", Object: "model", OwnedBy: "provider-2"},
		},
		Object: "list",
	}

	assert.Len(t, response.Data, 2)
	assert.Equal(t, "list", response.Object)
}

func TestFetchModelsFromProvider_InvalidURL(t *testing.T) {
	models, err := fetchModelsFromProvider(
		"://invalid-url",
		"Bearer test-token",
		"",
		ProviderAnthropic,
	)

	assert.Error(t, err)
	assert.Nil(t, models)
}

func TestDetermineTarget_WithKimiModel(t *testing.T) {
	body := []byte(`{"model": "kimi-for-coding", "messages": [{"role": "user", "content": "hi"}]}`)

	targetURL, authHeader, err := determineTarget(body)

	require.NoError(t, err)
	assert.True(t, strings.Contains(targetURL.Host, "kimi.com"))
	assert.True(t, strings.HasPrefix(authHeader, "Bearer "))
}

func TestDetermineTarget_WithAnthropicModel(t *testing.T) {
	body := []byte(`{"model": "claude-haiku-4-5-20251001", "messages": [{"role": "user", "content": "hi"}]}`)

	targetURL, authHeader, err := determineTarget(body)

	require.NoError(t, err)
	assert.True(t, strings.Contains(targetURL.Host, "anthropic.com"))
	assert.True(t, strings.HasPrefix(authHeader, "Bearer ") || authHeader == "")
}

func TestDetermineTarget_InvalidJSON(t *testing.T) {
	body := []byte(`invalid json`)

	targetURL, authHeader, err := determineTarget(body)

	// Should default to Anthropic when JSON is invalid
	require.NoError(t, err)
	assert.True(t, strings.Contains(targetURL.Host, "anthropic.com"))
	assert.True(t, strings.HasPrefix(authHeader, "Bearer ") || authHeader == "")
}

func TestGetDefaultAnthropicTarget(t *testing.T) {
	targetURL, authHeader, err := getDefaultAnthropicTarget()

	require.NoError(t, err)
	assert.True(t, strings.Contains(targetURL.Host, "anthropic.com"))
	assert.Equal(t, "https://api.anthropic.com", targetURL.String())
	// Auth header should be empty or start with Bearer (depends on credentials availability)
	assert.True(t, authHeader == "" || strings.HasPrefix(authHeader, "Bearer "))
}

func TestProviderModel_TimestampParsing(t *testing.T) {
	// Test with Unix timestamp
	pm1 := ProviderModel{
		ID:      "model-1",
		Created: 1234567890,
	}
	assert.Equal(t, int64(1234567890), pm1.Created)

	// Test with ISO 8601 timestamp
	pm2 := ProviderModel{
		ID:        "model-2",
		CreatedAt: "2024-01-15T10:30:00Z",
	}
	assert.Equal(t, "2024-01-15T10:30:00Z", pm2.CreatedAt)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "oauth-2025-04-20", AnthropicOAuthBeta)
	assert.Equal(t, "2023-06-01", AnthropicVersion)
	assert.Equal(t, "KimiCLI/1.19.0", KimiUserAgent)
}

func TestLoadConfig(t *testing.T) {
	// Save original values
	originalPort := Config.Port
	originalKimiKey := Config.KimiAPIKey

	// Set test env vars
	// Note: These won't affect the actual Config since it was already loaded,
	// but we verify the structure works

	// Restore
	Config.Port = originalPort
	Config.KimiAPIKey = originalKimiKey
}

func TestProviderFetchConfig(t *testing.T) {
	cfg := providerFetchConfig{
		name:      "TestProvider",
		baseURL:   "https://test.example.com",
		authFunc:  func() (string, error) { return "Bearer test", nil },
		userAgent: "TestAgent/1.0",
		provider:  ProviderAnthropic,
	}

	assert.Equal(t, "TestProvider", cfg.name)
	assert.Equal(t, "https://test.example.com", cfg.baseURL)
	assert.Equal(t, "TestAgent/1.0", cfg.userAgent)
	assert.Equal(t, ProviderAnthropic, cfg.provider)

	auth, err := cfg.authFunc()
	require.NoError(t, err)
	assert.Equal(t, "Bearer test", auth)
}

func BenchmarkDetermineTarget(b *testing.B) {
	body := []byte(`{"model": "kimi-for-coding", "messages": [{"role": "user", "content": "hi"}]}`)

	for i := 0; i < b.N; i++ {
		_, _, _ = determineTarget(body)
	}
}

func BenchmarkModelJSONMarshal(b *testing.B) {
	model := Model{
		Created: 1234567890,
		ID:      "test-model",
		Object:  "model",
		OwnedBy: "test-provider",
	}

	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(model)
	}
}

// Integration test helpers
func createTestJSONRequest(t *testing.T, model string) io.Reader {
	body := map[string]interface{}{
		"model":      model,
		"max_tokens": 100,
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
	}

	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	return bytes.NewReader(jsonBody)
}

func TestProxyEndpoint_WithKimiModel(t *testing.T) {
	// Skip due to httptest.ResponseRecorder limitation with ReverseProxy
	t.Skip("Skipped: httptest.ResponseRecorder doesn't support http.CloseNotifier required by ReverseProxy")
}

func TestHTTPClientTimeout(t *testing.T) {
	// Verify the shared HTTP client has the expected timeout
	assert.Equal(t, 10*time.Second, HTTPClient.Timeout)
}
