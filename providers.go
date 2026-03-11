package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// fetchModelsFromProvider fetches models from a provider's /v1/models endpoint
func fetchModelsFromProvider(baseURL, authHeader, userAgent string, provider ProviderType) ([]Model, error) {
	req, err := http.NewRequest("GET", baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", authHeader)
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	// Add Anthropic-specific headers for OAuth
	if provider == ProviderAnthropic {
		req.Header.Set("anthropic-beta", AnthropicOAuthBeta)
		req.Header.Set("anthropic-version", AnthropicVersion)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(body))
	}

	var providerResp ProviderModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&providerResp); err != nil {
		return nil, err
	}

	// Pre-allocate slice with known capacity
	models := make([]Model, 0, len(providerResp.Data))
	for _, pm := range providerResp.Data {
		model := Model{
			ID:      pm.ID,
			Object:  "model",
			OwnedBy: string(provider),
		}

		// Handle different timestamp formats
		switch {
		case pm.Created > 0:
			model.Created = pm.Created
		case pm.CreatedAt != "":
			if t, err := time.Parse(time.RFC3339, pm.CreatedAt); err == nil {
				model.Created = t.Unix()
			}
		}

		models = append(models, model)
	}

	return models, nil
}

func handleModels(c *gin.Context) {
	var allModels []Model
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Define provider configurations
	providers := []providerFetchConfig{
		{
			name:     "Anthropic",
			baseURL:  config.AnthropicBaseURL,
			authFunc: func() (string, error) { token, err := getClaudeToken(); return "Bearer " + token, err },
			provider: ProviderAnthropic,
		},
		{
			name:      "Kimi",
			baseURL:   config.KimiBaseURL,
			authFunc:  func() (string, error) { return "Bearer " + config.KimiAPIKey, nil },
			userAgent: KimiUserAgent,
			provider:  ProviderKimi,
		},
	}

	for _, p := range providers {
		wg.Add(1)
		go func(cfg providerFetchConfig) {
			defer wg.Done()

			authHeader, err := cfg.authFunc()
			if err != nil {
				fmt.Printf("  ⚠️ Failed to get %s credentials: %v\n", cfg.name, err)
				return
			}

			models, err := fetchModelsFromProvider(cfg.baseURL, authHeader, cfg.userAgent, cfg.provider)
			if err != nil {
				fmt.Printf("  ⚠️ Failed to fetch %s models: %v\n", cfg.name, err)
				return
			}

			mu.Lock()
			allModels = append(allModels, models...)
			mu.Unlock()
			fmt.Printf("  ✓ Fetched %d models from %s\n", len(models), cfg.name)
		}(p)
	}

	wg.Wait()

	response := ModelsResponse{
		Data:   allModels,
		Object: "list",
	}
	c.JSON(http.StatusOK, response)
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

// getDefaultAnthropicTarget returns the default Anthropic target for GET requests
func getDefaultAnthropicTarget() (*url.URL, string, error) {
	targetURL, err := url.Parse(config.AnthropicBaseURL)
	if err != nil {
		return nil, "", err
	}

	token, err := getClaudeToken()
	if err != nil {
		return nil, "", err
	}

	return targetURL, "Bearer " + token, nil
}
