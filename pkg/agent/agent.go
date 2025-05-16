package agent

import (
	"context"
	"fmt"
	"time"

	"llm-agent/pkg/models"
	"llm-agent/pkg/tools"
)

// Statistics tracks usage statistics for the agent
type Statistics struct {
	TotalTokens     int64
	StartTime       time.Time
	LastMessageTime time.Time
	MessageCount    int
}

// Agent represents a conversational agent that can use tools
type Agent struct {
	model        models.Model
	getUserInput func() (string, bool)
	tools        []tools.Tool
	showStats    bool
	stats        *Statistics
}

// NewAgent creates a new agent instance
func NewAgent(model models.Model, getUserInput func() (string, bool), tools []tools.Tool, showStats bool) *Agent {
	return &Agent{
		model:        model,
		getUserInput: getUserInput,
		tools:        tools,
		showStats:    showStats,
		stats: &Statistics{
			StartTime: time.Now(),
		},
	}
}

// Run starts the agent's conversation loop
func (a *Agent) Run(ctx context.Context) error {
	messages := []models.Message{}

	fmt.Println("Chat with", a.model.GetName(), "(use 'ctrl-c' to quit)")

	readUserInput := true
	for {
		if readUserInput {
			fmt.Print("\u001b[94mYou\u001b[0m: ")
			userInput, ok := a.getUserInput()
			if !ok {
				break
			}

			messages = append(messages, models.Message{
				Role:    "user",
				Content: userInput,
			})
			a.stats.MessageCount++
		}

		response, err := a.model.GenerateResponse(ctx, messages)
		if err != nil {
			return err
		}

		messages = append(messages, models.Message{
			Role:    "assistant",
			Content: response.Content,
		})

		a.stats.TotalTokens += response.Usage.InputTokens + response.Usage.OutputTokens
		a.stats.LastMessageTime = time.Now()

		fmt.Printf("\u001b[93m%s\u001b[0m: %s\n", a.model.GetName(), response.Content)

		if a.showStats {
			a.PrintResponseStats(response.Usage)
		}

		readUserInput = true
	}

	return nil
}

// PrintResponseStats prints statistics for a single response
func (a *Agent) PrintResponseStats(usage models.Usage) {
	fmt.Printf("\n\u001b[90mResponse Stats:\n")
	fmt.Printf("Input Tokens: %d\n", usage.InputTokens)
	fmt.Printf("Output Tokens: %d\n", usage.OutputTokens)
	fmt.Printf("Total Tokens: %d\u001b[0m\n\n", usage.InputTokens+usage.OutputTokens)
}

// PrintStats prints the overall session statistics
func (a *Agent) PrintStats() {
	duration := time.Since(a.stats.StartTime)
	tokensPerSecond := float64(a.stats.TotalTokens) / duration.Seconds()

	fmt.Printf("\n=== Session Statistics ===\n")
	fmt.Printf("Total Messages: %d\n", a.stats.MessageCount)
	fmt.Printf("Total Tokens: %d\n", a.stats.TotalTokens)
	fmt.Printf("Tokens per second: %.2f\n", tokensPerSecond)
	fmt.Printf("Session Duration: %s\n", duration.Round(time.Second))
	fmt.Printf("=======================\n")
}
