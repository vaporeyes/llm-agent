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

	"github.com/google/uuid"
)

// Version information
const (
	Version = "0.1.0"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBlue   = "\033[94m"       // Light blue
	colorGreen  = "\033[92m"       // Light green
	colorYellow = "\033[93m"       // Light yellow for tool usage
	colorOrange = "\033[38;5;208m" // Light orange for version and model info
)

// Spinner animation frames
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Statistics tracks usage statistics for the agent
type Statistics struct {
	TotalInputTokens  int64
	TotalOutputTokens int64
	StartTime         time.Time
	LastResponseTime  time.Duration
}

// Agent represents a chat agent that can interact with an LLM and use tools
type Agent struct {
	model         models.Model
	getUserInput  func() (string, bool)
	tools         []tools.Tool
	showStats     bool
	stats         Statistics
	storage       *storage.ChatStorage
	workspaceRoot string
}

// NewAgent creates a new agent with the given model and tools
func NewAgent(model models.Model, getUserInput func() (string, bool), tools []tools.Tool, showStats bool, storagePath string, workspaceRoot string) (*Agent, error) {
	chatStorage, err := storage.NewChatStorage(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat storage: %w", err)
	}

	// Set tools for the model
	if err := model.SetTools(tools); err != nil {
		return nil, fmt.Errorf("failed to set tools: %w", err)
	}

	return &Agent{
		model:        model,
		getUserInput: getUserInput,
		tools:        tools,
		showStats:    showStats,
		stats: Statistics{
			StartTime: time.Now(),
		},
		storage:       chatStorage,
		workspaceRoot: workspaceRoot,
	}, nil
}

// Run starts the agent's main loop
func (a *Agent) Run(ctx context.Context) error {
	// Print version and model information
	fmt.Printf("%sLLM Agent v%s using model: %s%s\n\n",
		colorOrange,
		Version,
		a.model.GetName(),
		colorReset)

	var messages []models.Message

	// Add system message to describe available tools
	toolDescriptions := make([]string, len(a.tools))
	for i, tool := range a.tools {
		toolDescriptions[i] = fmt.Sprintf("- %s: %s", tool.GetName(), tool.GetDescription())
	}

	var systemMessage string
	baseModelType := strings.Split(a.model.GetName(), "-")[0]
	if baseModelType == "ollama" {
		systemMessage = fmt.Sprintf(`<system>
You are a helpful AI assistant with access to the following tools:

%s

## Instructions for tool usage:
1. When a user asks you to perform a task requiring these tools, analyze which tool is appropriate.
2. To use a tool, format your response using JSON within <tool></tool> tags:
   <tool>
   {
     "name": "tool_name",
     "arguments": {
       "param1": "value1",
       "param2": "value2"
     }
   }
   </tool>

3. After each tool call, wait for the result which will appear in <result></result> tags.
4. Explain your reasoning both before and after using tools.
5. Present results clearly with appropriate formatting.

Remember that you're running locally with limited resources, so be efficient with your reasoning.
</system>`, strings.Join(toolDescriptions, "\n"))
	} else {
		systemMessage = fmt.Sprintf(`You are a helpful AI assistant with access to the following tools:

%s

When a user asks you to perform a task that can be done using these tools, you should use them. Always explain what you're doing and show the results of each tool usage. If a user asks a question that is not related to tool usage, answer the question as normal.`, strings.Join(toolDescriptions, "\n"))
	}
	messages = append(messages, models.Message{
		Role:    "system",
		Content: systemMessage,
	})

	// Initialize tools
	a.tools = []tools.Tool{
		tools.NewReadFileTool(),
		tools.NewListDirTool(a.workspaceRoot),
		tools.NewSearchFileTool(),
		tools.NewSummarizeFileTool(a.workspaceRoot),
	}

	// Set tools for the model
	if err := a.model.SetTools(a.tools); err != nil {
		return fmt.Errorf("failed to set tools: %w", err)
	}

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
		if strings.Contains(fullResponse, "<tool>") || strings.Contains(fullResponse, "[Tool:") || strings.Contains(fullResponse, "tool_calls") {
			//
			// Extract tool name and input
			lines := strings.Split(fullResponse, "\n")
			var toolName string
			var toolInput string

			// Try to parse as XML tool format first
			if toolStart := strings.Index(fullResponse, "<tool>"); toolStart != -1 {
				if toolEnd := strings.Index(fullResponse, "</tool>"); toolEnd != -1 {
					toolJSON := fullResponse[toolStart+6 : toolEnd]
					var toolCall struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}
					if err := json.Unmarshal([]byte(toolJSON), &toolCall); err == nil {

						toolName = toolCall.Name
						toolInput = string(toolCall.Arguments)
					} else {

					}
				}
			}

			// If XML parsing failed, try Ollama's tool_calls format if the response looks like JSON
			if toolName == "" && strings.TrimSpace(fullResponse)[0] == '{' {
				var toolCall struct {
					Function struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				}
				if err := json.Unmarshal([]byte(fullResponse), &toolCall); err == nil {

					toolName = toolCall.Function.Name
					toolInput = string(toolCall.Function.Arguments)
				} else {

				}
			}

			// If both XML and Ollama parsing failed, try Claude's format
			if toolName == "" {
				for i, line := range lines {
					if strings.HasPrefix(line, "[Tool:") {
						toolName = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(line, "]"), "[Tool:"))
						if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "Input:") {
							toolInput = strings.TrimSpace(strings.TrimPrefix(lines[i+1], "Input:"))
						}
						break
					}
				}
			}

			if toolName != "" {
				for _, tool := range a.tools {
					if tool.GetName() == toolName {

						// Start spinner in a goroutine
						done := make(chan bool)
						go func() {
							i := 0
							for {
								select {
								case <-done:
									return
								default:
									fmt.Printf("\r%s%s%s", colorYellow, spinnerFrames[i], colorReset)
									i = (i + 1) % len(spinnerFrames)
									time.Sleep(100 * time.Millisecond)
								}
							}
						}()

						result, err := tool.Execute(json.RawMessage(toolInput))
						done <- true    // Stop the spinner
						fmt.Print("\r") // Clear the spinner line

						if err != nil {
							return fmt.Errorf("error executing tool %s: %w", toolName, err)
						}

						// Print tool result in yellow
						fmt.Printf("%s<result>%s</result>%s\n", colorYellow, result, colorReset)

						// Add tool result to messages
						messages = append(messages, models.Message{
							Role:    "assistant",
							Content: fullResponse,
						})
						messages = append(messages, models.Message{
							Role:    "user",
							Content: fmt.Sprintf("<result>%s</result>", result),
						})

						// Get model's response to the tool result
						fmt.Printf("%sAssistant: %s", colorGreen, colorReset)
						err = a.model.StreamResponse(ctx, messages, func(chunk string) error {
							// Color tool usage in yellow
							if strings.Contains(chunk, "<tool>") || strings.Contains(chunk, "<result>") || strings.Contains(chunk, "[Tool:") || strings.Contains(chunk, "tool_calls") {
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

		// Add a newline before the next user input
		fmt.Println()
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
