package bot

import (
	"github.com/sgx-labs/same-telegram/internal/ai"
)

// newAPIClient creates an AI API client for the given backend and key.
func newAPIClient(backend, apiKey string) (ai.Client, error) {
	return ai.NewClient(ai.Backend(backend), apiKey)
}

// defaultModelForBackend returns the default model name for a backend.
func defaultModelForBackend(backend string) string {
	return ai.DefaultModel(ai.Backend(backend))
}
