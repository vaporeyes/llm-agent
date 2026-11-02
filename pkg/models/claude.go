package models

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-agent/pkg/tools"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
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

	client := anthropic.NewClient(option.WithAPIKey(config.APIKey), option.WithHTTPClient(newHTTPClient()))
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

		claudeTools[i] = anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        tool.GetName(),
			Description: anthropic.String(tool.GetDescription()),
			InputSchema: inputSchema,
		}}
	}

	m.tools = claudeTools
	return nil
}

// toAnthropicMessages maps our messages onto Claude's roles. System messages
// become the system prompt, and consecutive tool results are merged into a
// single user turn as the API requires.
func toAnthropicMessages(messages []Message) ([]anthropic.TextBlockParam, []anthropic.MessageParam) {
	var system []anthropic.TextBlockParam
	var out []anthropic.MessageParam
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			system = append(system, anthropic.TextBlockParam{Text: msg.Content})
		case "assistant":
			var blocks []anthropic.ContentBlockParamUnion
			if msg.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
			}
			for _, call := range msg.ToolCalls {
				blocks = append(blocks, anthropic.ContentBlockParamOfRequestToolUseBlock(call.ID, call.Input, call.Name))
			}
			if len(blocks) > 0 {
				out = append(out, anthropic.NewAssistantMessage(blocks...))
			}
		case "tool":
			block := anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false)
			if n := len(out); n > 0 && out[n-1].Role == anthropic.MessageParamRoleUser {
				out[n-1].Content = append(out[n-1].Content, block)
			} else {
				out = append(out, anthropic.NewUserMessage(block))
			}
		default:
			out = append(out, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		}
	}
	return system, out
}

func (m *ClaudeModel) StreamResponse(ctx context.Context, messages []Message, onChunk func(chunk string) error) (*Response, error) {
	system, anthropicMessages := toAnthropicMessages(messages)

	stream := m.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:       anthropic.Model(m.config.ModelName),
		MaxTokens:   int64(m.config.MaxTokens),
		Temperature: anthropic.Float(m.config.Temperature),
		System:      system,
		Messages:    anthropicMessages,
		Tools:       m.tools,
	})
	defer stream.Close()

	var message anthropic.Message
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return nil, fmt.Errorf("failed to accumulate stream: %w", err)
		}
		if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
			if err := onChunk(event.Delta.Text); err != nil {
				return nil, fmt.Errorf("error processing chunk: %w", err)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}

	// Extract content from the response
	resp := &Response{
		Usage: Usage{
			InputTokens:  message.Usage.InputTokens,
			OutputTokens: message.Usage.OutputTokens,
		},
	}
	for _, block := range message.Content {
		switch block.Type {
		case "text":
			resp.Content += block.Text
		case "tool_use":
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Input: block.Input})
		}
	}
	return resp, nil
}

func (m *ClaudeModel) GetName() string {
	return fmt.Sprintf("claude-%s", m.config.ModelName)
}

func (m *ClaudeModel) GetMaxTokens() int {
	return m.config.MaxTokens
}
