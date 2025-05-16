# LLM Agent

Borrowed from ideas from [How to Build an AI Agent](https://ampcode.com/how-to-build-an-agent) followed by using [Cursor](cursor.com) to flesh it out a little more. The following is the LLM-generated but edited README:

A modular, extensible chat agent that interfaces with various LLM providers (Claude and Ollama) and provides a set of tools for file operations.

## Features

- 🤖 Multiple LLM model support:
  - Claude (via API)
  - Ollama (local models like llama2, mistral)
- 🛠️ Built-in tools for file operations:
  - Read file contents
  - List files and directories
  - Edit file contents
- 📊 Usage statistics tracking
- 🔄 Graceful shutdown handling
- 🎨 Colored terminal output
- ⚡ Streaming responses for real-time output

## Prerequisites

- Go 1.21 or later
- For Claude: Anthropic API key
- For Ollama: [Ollama](https://ollama.ai) installed and running locally

## Installation

1. Clone the repository:

```bash
git clone https://github.com/vaporeyes/llm-agent.git
cd llm-agent
```

2. Set up your API key (if using Claude):

```bash
export ANTHROPIC_API_KEY=your-api-key
```

3. Install Ollama (if using local models):

```bash
# macOS
brew install ollama

# Linux
curl https://ollama.ai/install.sh | sh
```

4. Pull a model (e.g., llama2):

```bash
ollama pull llama2
```

## Usage

### Build and Run

Build the project:

```bash
go build -o llm-agent ./cmd/agent
```

Run the agent:

```bash
./llm-agent
```

Or run directly with Go:

```bash
go run ./cmd/agent
```

### Command Line Options

- `-stats`: Show token usage statistics after each response and when exiting
- `-model`: Select the model to use ("claude" or "ollama")
- `-ollama-model`: Select the Ollama model to use (e.g., "llama2", "mistral")

Examples:

```bash
# Use Claude with streaming responses
./llm-agent -stats -model claude

# Use Ollama with llama2 and streaming responses
./llm-agent -stats -model ollama -ollama-model llama2

# Use Ollama with mistral and streaming responses
./llm-agent -stats -model ollama -ollama-model mistral
```

## Project Structure

```text
.
├── cmd
│   └── agent
│       └── main.go
├── go.mod
├── LICENSE
├── pkg
│   ├── agent
│   │   └── agent.go
│   ├── config
│   ├── models
│   │   ├── claude.go
│   │   ├── model.go
│   │   └── ollama.go
│   └── tools
│       ├── file_tools.go
│       └── tool.go
└── README.md
```
