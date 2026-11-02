package agent

import (
	"context"
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

// maxToolIterations bounds how many model calls one user message can trigger
const maxToolIterations = 10

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
	model        models.Model
	getUserInput func() (string, bool)
	tools        []tools.Tool
	toolsByName  map[string]tools.Tool
	showStats    bool
	stats        Statistics
	storage      *storage.ChatStorage
}

// NewAgent creates a new agent with the given model and tools
func NewAgent(model models.Model, getUserInput func() (string, bool), agentTools []tools.Tool, showStats bool, storagePath string) (*Agent, error) {
	chatStorage, err := storage.NewChatStorage(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat storage: %w", err)
	}

	// Set tools for the model
	if err := model.SetTools(agentTools); err != nil {
		return nil, fmt.Errorf("failed to set tools: %w", err)
	}

	toolsByName := make(map[string]tools.Tool, len(agentTools))
	for _, tool := range agentTools {
		toolsByName[tool.GetName()] = tool
	}

	return &Agent{
		model:        model,
		getUserInput: getUserInput,
		tools:        agentTools,
		toolsByName:  toolsByName,
		showStats:    showStats,
		stats: Statistics{
			StartTime: time.Now(),
		},
		storage: chatStorage,
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

	// Add system message to describe available tools
	messages := []models.Message{{
		Role:    "system",
		Content: buildSystemPrompt(a.model.GetName(), a.tools),
	}}

	// One conversation ID links every message in this session
	conversationID := uuid.New().String()

turns:
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

		// Add user message to history, remembering where this turn starts
		turnStart := len(messages)
		userMsg := models.Message{
			Role:    "user",
			Content: input,
		}
		messages = append(messages, userMsg)

		// Save user message
		a.save(userMsg, models.Usage{InputTokens: int64(len(strings.Fields(input)))}, conversationID)

		// Get model response
		startTime := time.Now()
		var fullResponse string

		// Let the model call tools until it answers without one or hits the limit
		for i := 0; i < maxToolIterations; i++ {
			fmt.Printf("%sAssistant: %s", colorGreen, colorReset)
			resp, err := a.model.StreamResponse(ctx, messages, func(chunk string) error {
				fmt.Print(chunk)
				return nil
			})
			if err != nil {
				// Drop the unanswered turn so the history stays valid for the next request
				fmt.Printf("\nError getting model response: %v\n\n", err)
				messages = messages[:turnStart]
				continue turns
			}
			fmt.Println()
			fullResponse += resp.Content

			// Add assistant response to history
			assistantMsg := models.Message{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			}
			messages = append(messages, assistantMsg)
			a.save(assistantMsg, models.Usage{OutputTokens: int64(len(strings.Fields(resp.Content)))}, conversationID)

			if len(resp.ToolCalls) == 0 {
				break
			}

			for _, call := range resp.ToolCalls {
				result := a.executeTool(call)

				// Add tool result to messages
				toolMsg := models.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: call.ID,
				}
				messages = append(messages, toolMsg)
				a.save(toolMsg, models.Usage{}, conversationID)
			}

			if i == maxToolIterations-1 {
				fmt.Printf("%s[stopped after %d tool iterations]%s\n", colorYellow, maxToolIterations, colorReset)
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

			fmt.Printf("\n%s[Stats] Response time: %v, Input tokens: %d, Output tokens: %d%s\n",
				colorYellow,
				a.stats.LastResponseTime.Round(time.Millisecond),
				int64(inputTokens),
				int64(outputTokens),
				colorReset)
		}

		// Add a newline before the next user input
		fmt.Println()
	}
}

// save records a message in chat storage, warning rather than failing on error
func (a *Agent) save(msg models.Message, usage models.Usage, conversationID string) {
	if err := a.storage.SaveMessage(msg, a.model.GetName(), usage, conversationID); err != nil {
		fmt.Printf("Warning: failed to save %s message: %v\n", msg.Role, err)
	}
}

// executeTool runs one tool call, showing a spinner while it works.
// Failures are returned as an error message for the model rather than a Go error,
// because every tool call must be answered for the conversation to stay valid
// and the model can often recover, for example by fixing a mistyped path.
func (a *Agent) executeTool(call models.ToolCall) string {
	fmt.Printf("%s[Tool: %s] %s%s\n", colorYellow, call.Name, string(call.Input), colorReset)

	if call.Name == "" {
		return `error: malformed tool call, expected <tool>{"name": "...", "arguments": {...}}</tool>`
	}
	tool, ok := a.toolsByName[call.Name]
	if !ok {
		return fmt.Sprintf("error: unknown tool %q", call.Name)
	}

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

	result, err := tool.Execute(call.Input)
	done <- true    // Stop the spinner
	fmt.Print("\r") // Clear the spinner line
	if err != nil {
		result = fmt.Sprintf("error: %v", err)
	}

	// Print tool result in yellow
	fmt.Printf("%s<result>%s</result>%s\n", colorYellow, result, colorReset)
	return result
}

// buildSystemPrompt describes the available tools in the format the model family expects
func buildSystemPrompt(modelName string, agentTools []tools.Tool) string {
	toolDescriptions := make([]string, len(agentTools))
	for i, tool := range agentTools {
		toolDescriptions[i] = fmt.Sprintf("- %s: %s", tool.GetName(), tool.GetDescription())
	}

	baseModelType := strings.Split(modelName, "-")[0]
	if baseModelType == "ollama" {
		return fmt.Sprintf(`<system>
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
	}
	return fmt.Sprintf(`You are a helpful AI assistant with access to the following tools:

%s

When a user asks you to perform a task that can be done using these tools, you should use them. Always explain what you're doing and show the results of each tool usage. If a user asks a question that is not related to tool usage, answer the question as normal.`, strings.Join(toolDescriptions, "\n"))
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
