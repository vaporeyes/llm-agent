package models

import (
	"context"
)

// Message represents a single message in a conversation
type Message struct {
	Role    string
	Content string
}

// Usage represents token usage statistics
type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

// Response represents a model's response
type Response struct {
	Content string
	Usage   Usage
}

// Model defines the interface that all LLM implementations must satisfy
type Model interface {
	// GenerateResponse takes a context and conversation history, returns a response
	GenerateResponse(ctx context.Context, messages []Message) (*Response, error)

	// GetName returns the name of the model
	GetName() string

	// GetMaxTokens returns the maximum number of tokens this model can handle
	GetMaxTokens() int
}

// ModelConfig holds configuration for a model
type ModelConfig struct {
	APIKey      string
	ModelName   string
	MaxTokens   int
	Temperature float64
}
