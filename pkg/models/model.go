// ABOUTME: Provider-neutral message, tool call, and response types plus the Model interface.
// ABOUTME: Also holds the HTTP client configuration shared by all providers.
package models

import (
	"context"
	"encoding/json"
	"llm-agent/pkg/tools"
	"net/http"
	"time"
)

// responseHeaderTimeout bounds how long a provider may take to start responding,
// which for local models includes loading them into memory. Streaming bodies get no
// total deadline because long generations are normal; a stalled stream is stopped by
// cancelling the request context.
const responseHeaderTimeout = 5 * time.Minute

// newHTTPClient returns the HTTP client shared by all providers
func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	return &http.Client{Transport: transport}
}

// Message represents a chat message.
// Role is one of "system", "user", "assistant", or "tool".
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ToolCalls holds the tools an assistant message requested
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID links a "tool" message to the call it answers
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolCall is a provider-neutral request from the model to run a tool
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// Usage represents token usage statistics
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// Response represents a model's response
type Response struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage"`
}

// ModelConfig contains configuration for a model
type ModelConfig struct {
	APIKey string
	// BaseURL overrides the provider endpoint; currently used by Ollama
	BaseURL     string
	ModelName   string
	MaxTokens   int
	Temperature float64
}

// Model defines the interface for different LLM models
type Model interface {
	// StreamResponse streams text chunks to onChunk and returns the complete
	// response, including any tool calls the model requested
	StreamResponse(ctx context.Context, messages []Message, onChunk func(chunk string) error) (*Response, error)

	// GetName returns the name of the model
	GetName() string

	// GetMaxTokens returns the maximum number of tokens the model can generate
	GetMaxTokens() int

	// SetTools sets the available tools for the model
	SetTools(tools []tools.Tool) error
}
