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
	modelType := flag.String("model", "claude", "Model to use (claude, chatgpt, ollama)")
	ollamaModel := flag.String("ollama-model", "llama2", "Model to use with Ollama (e.g., llama2, mistral)")
	claudeModel := flag.String("claude-model", "claude-3-sonnet-20240229", "Model to use with Claude (e.g., claude-3-sonnet-20240229, claude-3-opus-20240229)")
	chatgptModel := flag.String("chatgpt-model", "gpt-3.5-turbo", "Model to use with ChatGPT (e.g., gpt-3.5-turbo, gpt-4)")
	storagePath := flag.String("storage", "chat_history.json", "Path to store chat history")
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
			ModelName:   *claudeModel,
			MaxTokens:   1024,
			Temperature: 0.7,
		})
	case "chatgpt":
		if os.Getenv("OPENAI_API_KEY") == "" {
			fmt.Println("Error: OPENAI_API_KEY environment variable is not set")
			fmt.Println("Please set your API key using:")
			fmt.Println("  export OPENAI_API_KEY=your-api-key")
			os.Exit(1)
		}
		model, err = models.NewChatGPTModel(models.ModelConfig{
			APIKey:      os.Getenv("OPENAI_API_KEY"),
			ModelName:   *chatgptModel,
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
		tools.NewSearchFileTool(),
		tools.NewFindFileTool(),
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
	agent, err := agent.NewAgent(model, getUserInput, availableTools, *showStats, *storagePath)
	if err != nil {
		fmt.Printf("Error creating agent: %v\n", err)
		os.Exit(1)
	}

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
