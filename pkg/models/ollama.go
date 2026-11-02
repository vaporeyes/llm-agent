package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-agent/pkg/tools"
)

type OllamaModel struct {
	config ModelConfig
	client *http.Client
	tools  []tools.Tool
}

type ollamaRequest struct {
	Model    string                 `json:"model"`
	Messages []message              `json:"messages"`
	Stream   bool                   `json:"stream"`
	Tools    []toolParam            `json:"tools,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

type toolParam struct {
	Type     string        `json:"type"`
	Function functionParam `json:"function"`
}

type functionParam struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
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

func (m *OllamaModel) StreamResponse(ctx context.Context, messages []Message, onChunk func(chunk string) error) (*Response, error) {
	// Convert our messages to Ollama format
	ollamaMessages := make([]message, len(messages))
	for i, msg := range messages {
		ollamaMessages[i] = message{
			Role:    msg.Role,
			Content: msg.Content,
		}
		for _, call := range msg.ToolCalls {
			ollamaMessages[i].ToolCalls = append(ollamaMessages[i].ToolCalls, toolCall{
				ID:       call.ID,
				Type:     "function",
				Function: functionCall{Name: call.Name, Arguments: call.Input},
			})
		}
	}

	// Convert tools to Ollama format
	var ollamaTools []toolParam
	if m.tools != nil {
		ollamaTools = make([]toolParam, len(m.tools))
		for i, tool := range m.tools {
			// Parse the input schema
			var schema map[string]interface{}
			if err := json.Unmarshal(tool.GetInputSchema(), &schema); err != nil {
				return nil, fmt.Errorf("failed to parse tool schema: %w", err)
			}

			// Create parameters object in Ollama's format
			parameters := map[string]interface{}{
				"type":       "object",
				"properties": schema["properties"],
			}
			if required, ok := schema["required"].([]interface{}); ok {
				parameters["required"] = required
			}

			ollamaTools[i] = toolParam{
				Type: "function",
				Function: functionParam{
					Name:        tool.GetName(),
					Description: tool.GetDescription(),
					Parameters:  parameters,
				},
			}
		}
	}

	// Prepare request
	reqBody := ollamaRequest{
		Model:    m.config.ModelName,
		Messages: ollamaMessages,
		Stream:   true,
		Tools:    ollamaTools,
		Options: map[string]interface{}{
			"temperature": m.config.Temperature,
			"num_predict": m.config.MaxTokens,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	//

	// Make request to Ollama
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:11434/api/chat", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to Ollama: %w", err)
	}
	//
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API returned status code: %d", resp.StatusCode)
	}

	// Read the raw response for debugging
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Create a new reader with the raw body
	reader := bytes.NewReader(rawBody)
	decoder := json.NewDecoder(reader)

	out := &Response{}
	for {
		var ollamaResp ollamaResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Collect every native tool call the model makes
		for _, call := range ollamaResp.Message.ToolCalls {
			id := call.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", len(out.ToolCalls))
			}
			out.ToolCalls = append(out.ToolCalls, ToolCall{ID: id, Name: call.Function.Name, Input: call.Function.Arguments})
		}

		if ollamaResp.Message.Content != "" {
			// Handle regular message content
			out.Content += ollamaResp.Message.Content
			if err := onChunk(ollamaResp.Message.Content); err != nil {
				return nil, fmt.Errorf("error processing chunk: %w", err)
			}
		}

		if ollamaResp.Done {
			out.Usage = Usage{
				InputTokens:  int64(ollamaResp.PromptEvalCount),
				OutputTokens: int64(ollamaResp.EvalCount),
			}
			break
		}
	}

	// Models that ignore native tool calling may follow the <tool> text protocol
	// described in the system prompt instead
	if len(out.ToolCalls) == 0 {
		if call, ok := parseTextToolCall(out.Content); ok {
			out.ToolCalls = append(out.ToolCalls, call)
		}
	}

	return out, nil
}

// parseTextToolCall extracts a tool call written as JSON inside <tool></tool> tags.
// A tag pair with invalid JSON yields a call with an empty name so the agent can
// report the malformed call back to the model.
func parseTextToolCall(content string) (ToolCall, bool) {
	start := strings.Index(content, "<tool>")
	if start == -1 {
		return ToolCall{}, false
	}
	end := strings.Index(content[start:], "</tool>")
	if end == -1 {
		return ToolCall{}, false
	}
	raw := content[start+len("<tool>") : start+end]
	var parsed struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ToolCall{ID: "call_0", Input: json.RawMessage(raw)}, true
	}
	return ToolCall{ID: "call_0", Name: parsed.Name, Input: parsed.Arguments}, true
}

func (m *OllamaModel) SetTools(tools []tools.Tool) error {
	m.tools = tools
	return nil
}

func (m *OllamaModel) GetName() string {
	return fmt.Sprintf("ollama-%s", m.config.ModelName)
}

func (m *OllamaModel) GetMaxTokens() int {
	return m.config.MaxTokens
}
