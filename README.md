# LLM Agent

A modular, extensible chat agent that interfaces with various LLM providers (currently Claude) and provides a set of tools for file operations.

## Features

- 🤖 Multiple LLM model support (Claude, with extensible interface for others)
- 🛠️ Built-in tools for file operations:
  - Read file contents
  - List files and directories
  - Edit file contents
- 📊 Usage statistics tracking
- 🔄 Graceful shutdown handling
- 🎨 Colored terminal output

## Prerequisites

- Go 1.21 or later
- Anthropic API key (for Claude)

## Installation

1. Clone the repository:
```bash
git clone https://github.com/vaporeyes/llm-agent.git
cd llm-agent
```

2. Set up your API key:
```bash
export ANTHROPIC_API_KEY=your-api-key
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
- `-model`: Select the model to use (currently only "claude" is supported)

Example:
```bash
./llm-agent -stats -model claude
```

## Project Structure

```
.
├── cmd/
│   └── agent/          # Main application entry point
├── pkg/
│   ├── agent/          # Core agent implementation
│   ├── models/         # LLM model interfaces and implementations
│   └── tools/          # Tool interfaces and implementations
└── go.mod              # Go module definition
```

## Adding New Features

### Adding a New Model

1. Create a new file in `pkg/models/` (e.g., `gpt4.go`)
2. Implement the `Model` interface
3. Add the model type to the switch statement in `cmd/agent/main.go`

### Adding a New Tool

1. Create a new file in `pkg/tools/` (e.g., `web_tools.go`)
2. Implement the `Tool` interface (or use `BaseTool`)
3. Add the tool to the `availableTools` slice in `cmd/agent/main.go`

## License

MIT License 