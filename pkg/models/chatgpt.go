package models

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"llm-agent/pkg/tools"

	openai "github.com/sashabaranov/go-openai"
)

type ChatGPTModel struct {
	client *openai.Client
	config ModelConfig
	tools  []openai.Tool
}

func NewChatGPTModel(config ModelConfig) (*ChatGPTModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required for ChatGPT model")
	}

	client := openai.NewClient(config.APIKey)
	return &ChatGPTModel{
		client: client,
		config: config,
	}, nil
}

func (m *ChatGPTModel) GenerateResponse(ctx context.Context, messages []Message) (*Response, error) {
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		role := msg.Role
		if role != "user" && role != "assistant" && role != "system" {
			role = "user"
		}
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    role,
			Content: msg.Content,
		}
	}

	resp, err := m.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       m.config.ModelName,
		Messages:    openaiMessages,
		MaxTokens:   m.config.MaxTokens,
		Temperature: float32(m.config.Temperature),
		Tools:       m.tools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	content := resp.Choices[0].Message.Content

	return &Response{
		Content: content,
		Usage: Usage{
			InputTokens:  int64(resp.Usage.PromptTokens),
			OutputTokens: int64(resp.Usage.CompletionTokens),
		},
	}, nil
}

func (m *ChatGPTModel) StreamResponse(ctx context.Context, messages []Message, onChunk func(chunk string) error) error {
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		role := msg.Role
		if role != "user" && role != "assistant" && role != "system" {
			role = "user"
		}
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    role,
			Content: msg.Content,
		}
	}

	stream, err := m.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:       m.config.ModelName,
		Messages:    openaiMessages,
		MaxTokens:   m.config.MaxTokens,
		Temperature: float32(m.config.Temperature),
		Tools:       m.tools,
	})
	if err != nil {
		return fmt.Errorf("failed to create chat completion stream: %w", err)
	}
	defer stream.Close()

	for {
		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error receiving stream: %w", err)
		}
		chunk := response.Choices[0].Delta.Content
		if chunk != "" {
			if err := onChunk(chunk); err != nil {
				return fmt.Errorf("error processing chunk: %w", err)
			}
		}
	}
	return nil
}

func (m *ChatGPTModel) GetName() string {
	return fmt.Sprintf("chatgpt-%s", m.config.ModelName)
}

func (m *ChatGPTModel) GetMaxTokens() int {
	return m.config.MaxTokens
}

func (m *ChatGPTModel) SetTools(tools []tools.Tool) error {
	// Convert our tools to ChatGPT's tool format
	chatGPTTools := make([]openai.Tool, len(tools))
	for i, tool := range tools {
		// Parse the input schema
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.GetInputSchema(), &schema); err != nil {
			return fmt.Errorf("failed to parse tool schema: %w", err)
		}

		chatGPTTools[i] = openai.Tool{
			Type: "function",
			Function: &openai.FunctionDefinition{
				Name:        tool.GetName(),
				Description: tool.GetDescription(),
				Parameters:  schema,
			},
		}
	}

	m.tools = chatGPTTools
	return nil
}
