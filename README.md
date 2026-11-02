# LLM Agent

Borrowed from ideas from [How to Build an AI Agent](https://ampcode.com/how-to-build-an-agent) followed by using [Cursor](cursor.com) to flesh it out a little more. The following is the LLM-generated but edited README:

A modular, extensible chat agent that interfaces with various LLM providers (Claude and Ollama) and provides a set of tools for file operations.

## Features

- 🤖 Multiple LLM model support:
  - Claude (via API)
  - ChatGPT (via API)
  - Ollama (local models like llama2, mistral)
- 🛠️ Built-in tools for file operations:
  - Read file contents
  - List files and directories
  - Edit file contents
- 📊 Usage statistics tracking
- 🔄 Graceful shutdown handling
- 🎨 Colored terminal output
- ⚡ Streaming responses for real-time output
- 📤 Export to JSON for processing elsewhere like [Datasette](https://datasette.io/)

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

2. Set up your API keys (if not using local models):

```bash
# claude
export ANTHROPIC_API_KEY=your-api-key
# chatgpt
export OPENAI_API_KEY=your-api-key
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
# Use Claude with streaming responses with the default model
./llm-agent -stats -model claude

# Use ChatGPT with streaming responses with the default model with no stats
./llm-agent -model chatgpt -chatgpt-model

# Use ChatGPT with a specific model
./llm-agent -model chatgpt -chatgpt-model gpt-4

# Use Ollama with llama2 and streaming responses
./llm-agent -stats -model ollama -ollama-model llama2

# Use Ollama with mistral and streaming responses
./llm-agent -stats -model ollama -ollama-model mistral

# Use Ollama with llama3.2 and exporting chat history to a file
./llm-agent -stats -model ollama -ollama-model llama3.2 -storage "llama32
```

## Chat history output structure

History is written as [JSON Lines](https://jsonlines.org/) (default `chat_history.jsonl`), one message per line, appended as the conversation happens. Assistant messages that call tools include `tool_calls`, and the matching `tool` messages carry `tool_call_id`.

```json
{"id":"99755c80-6d0e-4c40-92a4-1ac48b1fa4eb","conversation_id":"a468a39b-3601-49be-b44d-5cc1bdadda9b","role":"user","content":"What is in main.go?","timestamp":"2025-05-16T06:19:06.712554-05:00","model":"ollama-llama3.2","usage":{"input_tokens":6,"output_tokens":0}}
{"id":"649b84e8-1c50-45b2-91e6-8f25b9600d04","conversation_id":"a468a39b-3601-49be-b44d-5cc1bdadda9b","role":"assistant","content":"","timestamp":"2025-05-16T06:19:07.204211-05:00","model":"ollama-llama3.2","tool_calls":[{"id":"call_0","name":"read_file","input":{"path":"main.go"}}],"usage":{"input_tokens":0,"output_tokens":12}}
```

Load into Datasette with `sqlite-utils insert chat.db messages chat_history.jsonl --nl`. Files from older versions used a single JSON array; convert them with `jq -c '.[]' chat_history.json > chat_history.jsonl`.

## Project Structure

```text
├── cmd
│   └── agent
│       └── main.go
├── go.mod
├── go.sum
├── LICENSE
├── pkg
│   ├── agent
│   │   └── agent.go
│   ├── models
│   │   ├── chatgpt.go
│   │   ├── claude.go
│   │   ├── model.go
│   │   └── ollama.go
│   ├── storage
│   │   └── chat.go
│   └── tools
│       ├── file_tools.go
│       └── tool.go
└── README.md
```
