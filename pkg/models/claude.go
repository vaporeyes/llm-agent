package models

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

type ClaudeModel struct {
	client *anthropic.Client
	config ModelConfig
}

func NewClaudeModel(config ModelConfig) (*ClaudeModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required for Claude model")
	}

	client := anthropic.NewClient()
	return &ClaudeModel{
		client: &client,
		config: config,
	}, nil
}

func (m *ClaudeModel) GenerateResponse(ctx context.Context, messages []Message) (*Response, error) {
	anthropicMessages := make([]anthropic.MessageParam, len(messages))
	for i, msg := range messages {
		anthropicMessages[i] = anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content))
	}

	message, err := m.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude3_7SonnetLatest,
		MaxTokens: int64(m.config.MaxTokens),
		Messages:  anthropicMessages,
	})
	if err != nil {
		return nil, err
	}

	// Extract the text content from the response
	var content string
	for _, block := range message.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return &Response{
		Content: content,
		Usage: Usage{
			InputTokens:  message.Usage.InputTokens,
			OutputTokens: message.Usage.OutputTokens,
		},
	}, nil
}

func (m *ClaudeModel) StreamResponse(ctx context.Context, messages []Message, onChunk func(chunk string) error) error {
	// For now, use non-streaming and call onChunk once with full response
	response, err := m.GenerateResponse(ctx, messages)
	if err != nil {
		return err
	}
	return onChunk(response.Content)
}

func (m *ClaudeModel) GetName() string {
	return "claude"
}

func (m *ClaudeModel) GetMaxTokens() int {
	return m.config.MaxTokens
}
