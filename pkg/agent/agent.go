package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

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
	TotalResponseTime time.Duration
	Turns             int64
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
		input, ok, err := a.readInput(ctx)
		if err != nil {
			return err
		}
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
		// Token usage is reported by the provider with the response, not per user message
		a.save(userMsg, models.Usage{}, conversationID)

		// Get model response
		startTime := time.Now()
		var turnUsage models.Usage

		// Let the model call tools until it answers without one or hits the limit
		for i := 0; i < maxToolIterations; i++ {
			fmt.Printf("%sAssistant: %s", colorGreen, colorReset)
			resp, err := a.model.StreamResponse(ctx, messages, func(chunk string) error {
				fmt.Print(sanitizeForTerminal(chunk))
				return nil
			})
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				// Drop the unanswered turn so the history stays valid for the next request
				fmt.Printf("\nError getting model response: %v\n\n", err)
				messages = messages[:turnStart]
				continue turns
			}
			fmt.Println()
			turnUsage.InputTokens += resp.Usage.InputTokens
			turnUsage.OutputTokens += resp.Usage.OutputTokens

			// Add assistant response to history
			assistantMsg := models.Message{
				Role:      "assistant",
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			}
			messages = append(messages, assistantMsg)
			a.save(assistantMsg, resp.Usage, conversationID)

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
		a.stats.TotalResponseTime += a.stats.LastResponseTime
		a.stats.Turns++
		a.stats.TotalInputTokens += turnUsage.InputTokens
		a.stats.TotalOutputTokens += turnUsage.OutputTokens
		if a.showStats {
			fmt.Printf("\n%s[Stats] Response time: %v, Input tokens: %d, Output tokens: %d%s\n",
				colorYellow,
				a.stats.LastResponseTime.Round(time.Millisecond),
				turnUsage.InputTokens,
				turnUsage.OutputTokens,
				colorReset)
		}

		// Add a newline before the next user input
		fmt.Println()
	}
}

// readInput waits for the next line of user input or for ctx to be cancelled.
// Blocking reads such as bufio.Scanner cannot observe ctx, so the read runs in a
// goroutine; on cancellation it stays blocked on stdin until the process exits,
// and the buffered channel lets it finish without a receiver.
func (a *Agent) readInput(ctx context.Context) (string, bool, error) {
	type line struct {
		text string
		ok   bool
	}
	lines := make(chan line, 1)
	go func() {
		text, ok := a.getUserInput()
		lines <- line{text, ok}
	}()
	select {
	case <-ctx.Done():
		return "", false, ctx.Err()
	case l := <-lines:
		return l.text, l.ok, nil
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
	fmt.Printf("%s[Tool: %s] %s%s\n", colorYellow, sanitizeForTerminal(call.Name), sanitizeForTerminal(string(call.Input)), colorReset)

	if call.Name == "" {
		return `error: malformed tool call, expected <tool>{"name": "...", "arguments": {...}}</tool>`
	}
	tool, ok := a.toolsByName[call.Name]
	if !ok {
		return fmt.Sprintf("error: unknown tool %q", call.Name)
	}

	// Start spinner in a goroutine; stopSpinner waits for it to exit so it
	// cannot write over the result, and runs even if the tool panics
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i = (i + 1) % len(spinnerFrames) {
			fmt.Printf("\r%s%s%s", colorYellow, spinnerFrames[i], colorReset)
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()
	stopSpinner := func() {
		close(done) // Stop the spinner
		<-finished
		fmt.Print("\r") // Clear the spinner line
	}

	result, err := runTool(tool, call.Input, stopSpinner)
	if err != nil {
		result = fmt.Sprintf("error: %v", err)
	}

	// Print tool result in yellow
	fmt.Printf("%s<result>%s</result>%s\n", colorYellow, sanitizeForTerminal(result), colorReset)
	return result
}

// runTool executes tool and always calls stop afterwards, including on panic
func runTool(tool tools.Tool, input json.RawMessage, stop func()) (string, error) {
	defer stop()
	return tool.Execute(input)
}

// sanitizeForTerminal removes control characters other than newline and tab from
// text that came from the model or a file, so it cannot inject escape sequences
// that rewrite the screen, set the window title, or write to the clipboard
func sanitizeForTerminal(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
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
	var average time.Duration
	if a.stats.Turns > 0 {
		average = a.stats.TotalResponseTime / time.Duration(a.stats.Turns)
	}
	fmt.Printf("Average response time: %v\n", average.Round(time.Millisecond))
	fmt.Printf("==================\n")
}
