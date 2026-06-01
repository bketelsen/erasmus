// Package githubcopilot adapts GitHub Copilot API-compatible streams to Erasmus provider events.
package githubcopilot

import (
	"fmt"
	"net/http"
	"strings"

	"erasmus/packages/auth"
	"erasmus/packages/provider/openai"
	"erasmus/packages/provider/openaicodex"
)

const defaultBaseURL = "https://api.individual.githubcopilot.com"

// Config configures a GitHub Copilot provider adapter.
type Config struct {
	AccessToken string
	BaseURL     string
	HTTPClient  *http.Client
}

// NewChatCompletions creates a Copilot client for OpenAI Chat Completions-compatible models.
func NewChatCompletions(cfg Config) (*openai.Client, error) {
	if strings.TrimSpace(cfg.AccessToken) == "" {
		return nil, fmt.Errorf("github-copilot access token is required")
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return openai.New(openai.Config{
		APIKey:     cfg.AccessToken,
		BaseURL:    baseURL,
		Headers:    auth.GitHubCopilotStaticHeaders(),
		Provider:   "github-copilot",
		HTTPClient: cfg.HTTPClient,
	})
}

// NewResponses creates a Copilot client for OpenAI Responses-compatible models.
func NewResponses(cfg Config) (*openaicodex.Client, error) {
	if strings.TrimSpace(cfg.AccessToken) == "" {
		return nil, fmt.Errorf("github-copilot access token is required")
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return openaicodex.New(openaicodex.Config{
		AccessToken:           cfg.AccessToken,
		BaseURL:               baseURL + "/responses",
		Headers:               auth.GitHubCopilotStaticHeaders(),
		Provider:              "github-copilot",
		HTTPClient:            cfg.HTTPClient,
		AllowMissingAccountID: true,
		DisableCodexHeaders:   true,
	})
}
