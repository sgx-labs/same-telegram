package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const ollamaAPIURL = "http://localhost:11434/api/generate"

// OllamaClient implements Client for the local Ollama API.
type OllamaClient struct {
	httpClient *http.Client
}

// NewOllamaClient creates a new Ollama client (no API key needed).
func NewOllamaClient() *OllamaClient {
	return &OllamaClient{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// Chat sends a prompt to the local Ollama API and returns the response.
func (c *OllamaClient) Chat(ctx context.Context, prompt string, model string) (string, error) {
	if model == "" {
		model = DefaultModel(BackendOllama)
	}

	reqBody := ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ollamaAPIURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Ollama API call (is Ollama running?): %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama error (HTTP %d): %s", resp.StatusCode, truncateError(body))
	}

	var result ollamaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if result.Error != "" {
		return "", fmt.Errorf("Ollama error: %s", result.Error)
	}

	return StripThinkingTokens(result.Response), nil
}
