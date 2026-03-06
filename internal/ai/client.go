package ai

import (
	"context"
	"fmt"
	"time"
)

// DefaultTimeout is the timeout for all AI API calls.
const DefaultTimeout = 60 * time.Second

// Client is the interface for AI chat backends.
type Client interface {
	// Chat sends a prompt to the AI backend and returns the response text.
	Chat(ctx context.Context, prompt string, model string) (string, error)
}

// ValidateKey makes a lightweight API call to verify the key works.
// Returns nil if valid, an error describing the problem otherwise.
type KeyValidator interface {
	ValidateKey(ctx context.Context) error
}

// Backend represents a supported AI backend.
type Backend string

const (
	BackendClaude Backend = "claude"
	BackendOpenAI Backend = "openai"
	BackendGemini Backend = "gemini"
	BackendOllama Backend = "ollama"
)

// DefaultModel returns the default model for a given backend.
func DefaultModel(backend Backend) string {
	switch backend {
	case BackendClaude:
		return "claude-sonnet-4-20250514"
	case BackendOpenAI:
		return "gpt-4o"
	case BackendGemini:
		return "gemini-2.5-flash"
	case BackendOllama:
		return "qwen2.5-coder:7b"
	default:
		return ""
	}
}

// NewClient creates an AI client for the given backend and API key.
func NewClient(backend Backend, apiKey string) (Client, error) {
	switch backend {
	case BackendClaude:
		return NewClaudeClient(apiKey), nil
	case BackendOpenAI:
		return NewOpenAIClient(apiKey), nil
	case BackendGemini:
		return NewGeminiClient(apiKey), nil
	case BackendOllama:
		return NewOllamaClient(), nil
	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
}
