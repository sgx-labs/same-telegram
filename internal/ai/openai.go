package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const openaiAPIURL = "https://api.openai.com/v1/chat/completions"

// OpenAIClient implements Client for the OpenAI Chat Completions API.
type OpenAIClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIClient creates a new OpenAI API client.
func NewOpenAIClient(apiKey string) *OpenAIClient {
	return &OpenAIClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

type openaiRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat sends a prompt to the OpenAI API and returns the response.
func (c *OpenAIClient) Chat(ctx context.Context, prompt string, model string) (string, error) {
	if model == "" {
		model = DefaultModel(BackendOpenAI)
	}

	reqBody := openaiRequest{
		Model: model,
		Messages: []openaiMessage{
			{Role: "user", Content: prompt},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openaiAPIURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncateError(body))
	}

	var result openaiResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("API error: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	return StripThinkingTokens(result.Choices[0].Message.Content), nil
}

// ValidateKey makes a minimal API call to verify the key.
func (c *OpenAIClient) ValidateKey(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	_, err := c.Chat(ctx, "Say OK", DefaultModel(BackendOpenAI))
	if err != nil {
		return fmt.Errorf("key validation failed: %w", err)
	}
	return nil
}
