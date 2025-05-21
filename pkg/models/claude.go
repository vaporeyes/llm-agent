package models

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-agent/pkg/tools"

	"github.com/anthropics/anthropic-sdk-go"
)

type ClaudeModel struct {
	client *anthropic.Client
	config ModelConfig
	tools  []anthropic.ToolUnionParam
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

func (m *ClaudeModel) SetTools(tools []tools.Tool) error {
	// Convert our tools to Claude's tool format
	claudeTools := make([]anthropic.ToolUnionParam, len(tools))
	for i, tool := range tools {
		// Parse the input schema
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.GetInputSchema(), &schema); err != nil {
			return fmt.Errorf("failed to parse tool schema: %w", err)
		}

		// Create the tool input schema
		inputSchema := anthropic.ToolInputSchemaParam{
			Type:        "object",
			Properties:  schema["properties"],
			ExtraFields: make(map[string]interface{}),
		}

		// Add required fields if present
		if required, ok := schema["required"].([]interface{}); ok {
			inputSchema.ExtraFields["required"] = required
		}

		claudeTools[i] = anthropic.ToolUnionParamOfTool(
			inputSchema,
			tool.GetName(),
		)
	}

	m.tools = claudeTools
	return nil
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
		Tools:     m.tools,
	})
	if err != nil {
		return nil, err
	}

	// Extract content from the response
	var content string
	for _, block := range message.Content {
		switch block.Type {
		case "text":
			textBlock := block.AsResponseTextBlock()
			content += textBlock.Text
		case "tool_use":
			toolBlock := block.AsResponseToolUseBlock()
			// Format tool usage in a way that's easy to read
			content += fmt.Sprintf("\n[Tool: %s]\nInput: %s\n", toolBlock.Name, string(toolBlock.Input))
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
	anthropicMessages := make([]anthropic.MessageParam, len(messages))
	for i, msg := range messages {
		anthropicMessages[i] = anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content))
	}

	message, err := m.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude3_7SonnetLatest,
		MaxTokens: int64(m.config.MaxTokens),
		Messages:  anthropicMessages,
		Tools:     m.tools,
	})
	if err != nil {
		return err
	}

	// Extract content from the response
	var content string
	for _, block := range message.Content {
		switch block.Type {
		case "text":
			textBlock := block.AsResponseTextBlock()
			content += textBlock.Text
		case "tool_use":
			toolBlock := block.AsResponseToolUseBlock()
			// Format tool usage in a way that's easy to read
			content += fmt.Sprintf("\n[Tool: %s]\nInput: %s\n", toolBlock.Name, string(toolBlock.Input))
		}
	}

	return onChunk(content)
}

func (m *ClaudeModel) GetName() string {
	return fmt.Sprintf("claude-%s", m.config.ModelName)
}

func (m *ClaudeModel) GetMaxTokens() int {
	return m.config.MaxTokens
}
