package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type OllamaModel struct {
	config ModelConfig
	client *http.Client
}

type ollamaRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Model           string  `json:"model"`
	Message         message `json:"message"`
	Done            bool    `json:"done"`
	TotalDuration   int64   `json:"total_duration"`
	LoadDuration    int64   `json:"load_duration"`
	PromptEvalCount int     `json:"prompt_eval_count"`
	EvalCount       int     `json:"eval_count"`
	EvalDuration    int64   `json:"eval_duration"`
}

func NewOllamaModel(config ModelConfig) (*OllamaModel, error) {
	if config.ModelName == "" {
		config.ModelName = "llama2" // default model
	}

	return &OllamaModel{
		config: config,
		client: &http.Client{},
	}, nil
}

func (m *OllamaModel) GenerateResponse(ctx context.Context, messages []Message) (*Response, error) {
	var fullResponse string
	err := m.StreamResponse(ctx, messages, func(chunk string) error {
		fullResponse += chunk
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Estimate token usage (rough approximation)
	inputTokens := estimateTokens(messages)
	outputTokens := estimateTokens([]Message{{Content: fullResponse}})

	return &Response{
		Content: fullResponse,
		Usage: Usage{
			InputTokens:  int64(inputTokens),
			OutputTokens: int64(outputTokens),
		},
	}, nil
}

func (m *OllamaModel) StreamResponse(ctx context.Context, messages []Message, onChunk func(chunk string) error) error {
	// Convert our messages to Ollama format
	ollamaMessages := make([]message, len(messages))
	for i, msg := range messages {
		ollamaMessages[i] = message(msg)
	}

	// Prepare request
	reqBody := ollamaRequest{
		Model:    m.config.ModelName,
		Messages: ollamaMessages,
		Stream:   true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make request to Ollama
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:11434/api/chat", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama API returned status code: %d", resp.StatusCode)
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		var ollamaResp ollamaResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to decode response: %w", err)
		}

		if err := onChunk(ollamaResp.Message.Content); err != nil {
			return fmt.Errorf("error processing chunk: %w", err)
		}

		if ollamaResp.Done {
			break
		}
	}

	return nil
}

func (m *OllamaModel) GetName() string {
	return fmt.Sprintf("ollama-%s", m.config.ModelName)
}

func (m *OllamaModel) GetMaxTokens() int {
	return m.config.MaxTokens
}

// estimateTokens provides a rough estimate of token count
// This is a very basic implementation - in practice, you'd want to use a proper tokenizer
func estimateTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		// Rough estimate: 1 token ≈ 4 characters
		total += len(msg.Content) / 4
	}
	return total
}
