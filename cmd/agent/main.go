package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"llm-agent/pkg/agent"
	"llm-agent/pkg/models"
	"llm-agent/pkg/tools"
)

// maxInputBytes is the longest single line of user input accepted
const maxInputBytes = 4 * 1024 * 1024

func main() {
	showStats := flag.Bool("stats", false, "Show statistics when the program exits")
	modelType := flag.String("model", "claude", "Model to use (claude, chatgpt, ollama)")
	ollamaURL := flag.String("ollama-url", models.DefaultOllamaURL, "Base URL of the Ollama server")
	ollamaModel := flag.String("ollama-model", "llama2", "Model to use with Ollama (e.g., llama2, mistral)")
	claudeModel := flag.String("claude-model", "claude-sonnet-5-5", "Model to use with Claude (e.g., claude-sonnet-5-5, claude-opus-5-5)")
	chatgptModel := flag.String("chatgpt-model", "gpt-3.5-turbo", "Model to use with ChatGPT (e.g., gpt-3.5-turbo, gpt-4)")
	storagePath := flag.String("storage", "chat_history.jsonl", "Path to store chat history (JSON Lines)")
	workspaceRoot := flag.String("workspace", ".", "Workspace root directory")
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
			BaseURL:     *ollamaURL,
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

	// Initialize user input
	scanner := bufio.NewScanner(os.Stdin)
	// Allow long pasted input; the default 64 KiB limit ended the session silently
	scanner.Buffer(make([]byte, 0, 64*1024), maxInputBytes)
	getUserInput := func() (string, bool) {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			}
			return "", false
		}
		return scanner.Text(), true
	}

	// Create and run agent
	agent, err := agent.NewAgent(
		model,
		getUserInput,
		[]tools.Tool{
			tools.NewReadFileTool(*workspaceRoot),
			tools.NewListDirTool(*workspaceRoot),
			tools.NewSearchFileTool(*workspaceRoot),
			tools.NewSummarizeFileTool(*workspaceRoot),
		},
		*showStats,
		*storagePath,
	)
	if err != nil {
		fmt.Printf("Error creating agent: %v\n", err)
		os.Exit(1)
	}

	// Set up signal handling for graceful shutdown. The first signal cancels
	// ctx so Run can stop at a safe point; restoring default handling after it
	// lets a second signal terminate a stuck process.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stop()
	}()

	// Run blocks until input ends, an unrecoverable error occurs, or ctx is cancelled
	if err := agent.Run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Println("\nReceived interrupt signal, shutting down...")
		} else {
			fmt.Printf("Error: %s\n", err.Error())
		}
	}

	if *showStats {
		agent.PrintStats()
	}
}
