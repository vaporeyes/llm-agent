package models

import (
	"context"
	"llm-agent/pkg/tools"
)

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage represents token usage statistics
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// Response represents a model's response
type Response struct {
	Content string `json:"content"`
	Usage   Usage  `json:"usage"`
}

// ModelConfig contains configuration for a model
type ModelConfig struct {
	APIKey      string
	ModelName   string
	MaxTokens   int
	Temperature float64
}

// Model defines the interface for different LLM models
type Model interface {
	// GenerateResponse generates a complete response for the given messages
	GenerateResponse(ctx context.Context, messages []Message) (*Response, error)

	// StreamResponse streams the response for the given messages
	StreamResponse(ctx context.Context, messages []Message, onChunk func(chunk string) error) error

	// GetName returns the name of the model
	GetName() string

	// GetMaxTokens returns the maximum number of tokens the model can generate
	GetMaxTokens() int

	// SetTools sets the available tools for the model
	SetTools(tools []tools.Tool) error
}
