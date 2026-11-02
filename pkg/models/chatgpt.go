// ABOUTME: OpenAI ChatGPT provider implementing the Model interface over streaming completions.
// ABOUTME: Reassembles streamed tool call fragments into structured ToolCalls.
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

	clientConfig := openai.DefaultConfig(config.APIKey)
	clientConfig.HTTPClient = newHTTPClient()
	client := openai.NewClientWithConfig(clientConfig)
	return &ChatGPTModel{
		client: client,
		config: config,
	}, nil
}

// toOpenAIMessages maps our messages, including tool calls and results, to the OpenAI format
func toOpenAIMessages(messages []Message) []openai.ChatCompletionMessage {
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		out := openai.ChatCompletionMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		for _, call := range msg.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, openai.ToolCall{
				ID:       call.ID,
				Type:     openai.ToolTypeFunction,
				Function: openai.FunctionCall{Name: call.Name, Arguments: string(call.Input)},
			})
		}
		openaiMessages[i] = out
	}
	return openaiMessages
}

func (m *ChatGPTModel) StreamResponse(ctx context.Context, messages []Message, onChunk func(chunk string) error) (*Response, error) {
	stream, err := m.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:         m.config.ModelName,
		Messages:      toOpenAIMessages(messages),
		MaxTokens:     m.config.MaxTokens,
		Temperature:   float32(m.config.Temperature),
		Tools:         m.tools,
		StreamOptions: &openai.StreamOptions{IncludeUsage: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion stream: %w", err)
	}
	defer stream.Close()

	resp := &Response{}
	// Tool call fragments arrive spread across chunks, keyed by index
	var calls []*ToolCall
	var args []string
	for {
		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error receiving stream: %w", err)
		}
		if response.Usage != nil {
			resp.Usage = Usage{
				InputTokens:  int64(response.Usage.PromptTokens),
				OutputTokens: int64(response.Usage.CompletionTokens),
			}
		}
		// The usage chunk and some filtered chunks carry no choices
		if len(response.Choices) == 0 {
			continue
		}
		delta := response.Choices[0].Delta
		for _, tc := range delta.ToolCalls {
			idx := len(calls)
			if tc.Index != nil {
				idx = *tc.Index
			}
			for len(calls) <= idx {
				calls = append(calls, &ToolCall{})
				args = append(args, "")
			}
			if tc.ID != "" {
				calls[idx].ID = tc.ID
			}
			if tc.Function.Name != "" {
				calls[idx].Name = tc.Function.Name
			}
			args[idx] += tc.Function.Arguments
		}
		chunk := delta.Content
		if chunk != "" {
			resp.Content += chunk
			if err := onChunk(chunk); err != nil {
				return nil, fmt.Errorf("error processing chunk: %w", err)
			}
		}
	}
	for i, call := range calls {
		call.Input = json.RawMessage(args[i])
		resp.ToolCalls = append(resp.ToolCalls, *call)
	}
	return resp, nil
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
