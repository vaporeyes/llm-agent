package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"llm-agent/pkg/models"
	"llm-agent/pkg/storage"
	"llm-agent/pkg/tools"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBlue   = "\033[94m" // Light blue
	colorGreen  = "\033[92m" // Light green
	colorYellow = "\033[93m" // Light yellow for tool usage
)

// Statistics tracks usage statistics for the agent
type Statistics struct {
	TotalInputTokens  int64
	TotalOutputTokens int64
	StartTime         time.Time
	LastResponseTime  time.Duration
}

// Agent represents a chat agent that can interact with an LLM and use tools
type Agent struct {
	model        models.Model
	getUserInput func() (string, bool)
	tools        []tools.Tool
	showStats    bool
	stats        Statistics
	storage      *storage.ChatStorage
}

// NewAgent creates a new agent with the given model and tools
func NewAgent(model models.Model, getUserInput func() (string, bool), tools []tools.Tool, showStats bool, storagePath string) (*Agent, error) {
	chatStorage, err := storage.NewChatStorage(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat storage: %w", err)
	}

	// Convert our tools to Claude's tool format
	claudeTools := make([]anthropic.ToolUnionParam, len(tools))
	for i, tool := range tools {
		// Parse the input schema
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.GetInputSchema(), &schema); err != nil {
			return nil, fmt.Errorf("failed to parse tool schema: %w", err)
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

	// Set tools for Claude model
	if claudeModel, ok := model.(*models.ClaudeModel); ok {
		claudeModel.SetTools(claudeTools)
	}

	return &Agent{
		model:        model,
		getUserInput: getUserInput,
		tools:        tools,
		showStats:    showStats,
		stats: Statistics{
			StartTime: time.Now(),
		},
		storage: chatStorage,
	}, nil
}

// Run starts the agent's main loop
func (a *Agent) Run(ctx context.Context) error {
	var messages []models.Message

	// Add system message to describe available tools
	toolDescriptions := make([]string, len(a.tools))
	for i, tool := range a.tools {
		toolDescriptions[i] = fmt.Sprintf("- %s: %s", tool.GetName(), tool.GetDescription())
	}
	systemMessage := fmt.Sprintf(`You are a helpful AI assistant with access to the following tools:

%s

When a user asks you to perform a task that can be done using these tools, you should use them. Always explain what you're doing and show the results of each tool usage.`, strings.Join(toolDescriptions, "\n"))

	messages = append(messages, models.Message{
		Role:    "system",
		Content: systemMessage,
	})

	for {
		// Get user input
		fmt.Printf("%sYou: %s", colorBlue, colorReset)
		input, ok := a.getUserInput()
		if !ok {
			return nil
		}

		if input == "" {
			continue
		}

		// Generate a new conversation ID for this exchange
		conversationID := uuid.New().String()

		// Add user message to history
		userMsg := models.Message{
			Role:    "user",
			Content: input,
		}
		messages = append(messages, userMsg)

		// Save user message
		if err := a.storage.SaveMessage(userMsg, a.model.GetName(), models.Usage{
			InputTokens: int64(len(strings.Fields(input))),
		}, conversationID); err != nil {
			fmt.Printf("Warning: failed to save user message: %v\n", err)
		}

		// Get model response
		startTime := time.Now()
		fmt.Printf("%sAssistant: %s", colorGreen, colorReset)

		// Stream the response
		var fullResponse string
		err := a.model.StreamResponse(ctx, messages, func(chunk string) error {
			// Color tool usage in yellow
			if strings.Contains(chunk, "[Tool:") {
				fmt.Printf("%s%s%s", colorYellow, chunk, colorReset)
			} else {
				fmt.Print(chunk)
			}
			fullResponse += chunk
			return nil
		})
		if err != nil {
			return fmt.Errorf("error getting model response: %w", err)
		}

		// Check if the response contains tool usage
		if strings.Contains(fullResponse, "[Tool:") {
			// Extract tool name and input
			lines := strings.Split(fullResponse, "\n")
			var toolName string
			var toolInput string
			for i, line := range lines {
				if strings.HasPrefix(line, "[Tool:") {
					toolName = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(line, "]"), "[Tool:"))
					if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "Input:") {
						toolInput = strings.TrimSpace(strings.TrimPrefix(lines[i+1], "Input:"))
					}
					break
				}
			}

			// Find and execute the tool
			for _, tool := range a.tools {
				if tool.GetName() == toolName {
					result, err := tool.Execute(json.RawMessage(toolInput))
					if err != nil {
						return fmt.Errorf("error executing tool %s: %w", toolName, err)
					}

					// Add tool result to messages
					messages = append(messages, models.Message{
						Role:    "assistant",
						Content: fullResponse,
					})
					messages = append(messages, models.Message{
						Role:    "user",
						Content: fmt.Sprintf("Tool result: %s", result),
					})

					// Get model's response to the tool result
					fmt.Printf("%sAssistant: %s", colorGreen, colorReset)
					err = a.model.StreamResponse(ctx, messages, func(chunk string) error {
						// Color tool usage in yellow
						if strings.Contains(chunk, "[Tool:") {
							fmt.Printf("%s%s%s", colorYellow, chunk, colorReset)
						} else {
							fmt.Print(chunk)
						}
						fullResponse += chunk
						return nil
					})
					if err != nil {
						return fmt.Errorf("error getting model response to tool result: %w", err)
					}
					break
				}
			}
		}

		// Update statistics
		a.stats.LastResponseTime = time.Since(startTime)
		if a.showStats {
			// Estimate tokens for streaming response
			inputTokens := float64(len(strings.Fields(input))) * 1.3 // Rough estimate
			outputTokens := float64(len(strings.Fields(fullResponse))) * 1.3
			a.stats.TotalInputTokens += int64(inputTokens)
			a.stats.TotalOutputTokens += int64(outputTokens)

			fmt.Printf("\n\n%s[Stats] Response time: %v, Input tokens: %d, Output tokens: %d%s\n",
				colorYellow,
				a.stats.LastResponseTime.Round(time.Millisecond),
				int64(inputTokens),
				int64(outputTokens),
				colorReset)
		}

		// Add assistant response to history
		assistantMsg := models.Message{
			Role:    "assistant",
			Content: fullResponse,
		}
		messages = append(messages, assistantMsg)

		// Save assistant message with the same conversation ID
		if err := a.storage.SaveMessage(assistantMsg, a.model.GetName(), models.Usage{
			OutputTokens: int64(len(strings.Fields(fullResponse))),
		}, conversationID); err != nil {
			fmt.Printf("Warning: failed to save assistant message: %v\n", err)
		}
	}
}

// PrintStats prints the agent's statistics
func (a *Agent) PrintStats() {
	if !a.showStats {
		return
	}

	totalTime := time.Since(a.stats.StartTime)
	fmt.Printf("\n=== Statistics ===\n")
	fmt.Printf("Total runtime: %v\n", totalTime.Round(time.Second))
	fmt.Printf("Total input tokens: %d\n", a.stats.TotalInputTokens)
	fmt.Printf("Total output tokens: %d\n", a.stats.TotalOutputTokens)
	fmt.Printf("Average response time: %v\n", a.stats.LastResponseTime.Round(time.Millisecond))
	fmt.Printf("==================\n")
}
