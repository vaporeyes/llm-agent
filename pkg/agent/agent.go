package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"llm-agent/pkg/models"
	"llm-agent/pkg/tools"
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
}

// NewAgent creates a new agent with the given model and tools
func NewAgent(model models.Model, getUserInput func() (string, bool), tools []tools.Tool, showStats bool) *Agent {
	return &Agent{
		model:        model,
		getUserInput: getUserInput,
		tools:        tools,
		showStats:    showStats,
		stats: Statistics{
			StartTime: time.Now(),
		},
	}
}

// Run starts the agent's main loop
func (a *Agent) Run(ctx context.Context) error {
	var messages []models.Message

	for {
		// Get user input
		fmt.Print("\nYou: ")
		input, ok := a.getUserInput()
		if !ok {
			return nil
		}

		if input == "" {
			continue
		}

		// Add user message to history
		messages = append(messages, models.Message{
			Role:    "user",
			Content: input,
		})

		// Get model response
		startTime := time.Now()
		fmt.Print("\nAssistant: ")

		// Stream the response
		var fullResponse string
		err := a.model.StreamResponse(ctx, messages, func(chunk string) error {
			fmt.Print(chunk)
			fullResponse += chunk
			return nil
		})
		if err != nil {
			return fmt.Errorf("error getting model response: %w", err)
		}

		// Update statistics
		a.stats.LastResponseTime = time.Since(startTime)
		if a.showStats {
			// Estimate tokens for streaming response
			inputTokens := float64(len(strings.Fields(input))) * 1.3 // Rough estimate
			outputTokens := float64(len(strings.Fields(fullResponse))) * 1.3
			a.stats.TotalInputTokens += int64(inputTokens)
			a.stats.TotalOutputTokens += int64(outputTokens)

			fmt.Printf("\n\n[Stats] Response time: %v, Input tokens: %d, Output tokens: %d\n",
				a.stats.LastResponseTime.Round(time.Millisecond),
				int64(inputTokens),
				int64(outputTokens))
		}

		// Add assistant response to history
		messages = append(messages, models.Message{
			Role:    "assistant",
			Content: fullResponse,
		})
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
