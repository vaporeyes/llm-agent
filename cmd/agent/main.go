package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"llm-agent/pkg/agent"
	"llm-agent/pkg/models"
	"llm-agent/pkg/tools"
)

func main() {
	showStats := flag.Bool("stats", false, "Show statistics when the program exits")
	modelType := flag.String("model", "claude", "Model to use (claude, ollama)")
	ollamaModel := flag.String("ollama-model", "llama2", "Model to use with Ollama (e.g., llama2, mistral)")
	flag.Parse()

	// Initialize model
	var model models.Model
	var err error
	switch *modelType {
	case "claude":
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			fmt.Println("Error: ANTHROPIC_API_KEY environment variable is not set")
			fmt.Println("Please set your API key using:")
			fmt.Println("  export ANTHROPIC_API_KEY=your-api-key")
			os.Exit(1)
		}
		model, err = models.NewClaudeModel(models.ModelConfig{
			APIKey:      os.Getenv("ANTHROPIC_API_KEY"),
			ModelName:   "claude-3-sonnet-20240229",
			MaxTokens:   1024,
			Temperature: 0.7,
		})
	case "ollama":
		model, err = models.NewOllamaModel(models.ModelConfig{
			ModelName:   *ollamaModel,
			MaxTokens:   1024,
			Temperature: 0.7,
		})
	default:
		fmt.Printf("Error: Unknown model type %s\n", *modelType)
		os.Exit(1)
	}

	if err != nil {
		fmt.Printf("Error initializing model: %v\n", err)
		os.Exit(1)
	}

	// Initialize tools
	availableTools := []tools.Tool{
		tools.NewReadFileTool(),
		tools.NewListFilesTool(),
		tools.NewEditFileTool(),
	}

	// Initialize user input
	scanner := bufio.NewScanner(os.Stdin)
	getUserInput := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	// Create and run agent
	agent := agent.NewAgent(model, getUserInput, availableTools, *showStats)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run the agent in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- agent.Run(context.TODO())
	}()

	// Wait for either an error or a signal
	select {
	case err := <-errChan:
		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
		}
	case <-sigChan:
		fmt.Println("\nReceived interrupt signal, shutting down...")
	}

	if *showStats {
		agent.PrintStats()
	}
}
