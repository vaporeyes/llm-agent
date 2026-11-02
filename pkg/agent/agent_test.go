// ABOUTME: Tests for the agent loop, system prompt construction, and tool dispatch.
// ABOUTME: Uses a scripted fake model so no network access is required.
package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"llm-agent/pkg/models"
	"llm-agent/pkg/tools"
)

type fakeModel struct {
	name     string
	setTools []tools.Tool
}

func (m *fakeModel) GenerateResponse(ctx context.Context, messages []models.Message) (*models.Response, error) {
	return &models.Response{}, nil
}

func (m *fakeModel) StreamResponse(ctx context.Context, messages []models.Message, onChunk func(string) error) error {
	return nil
}

func (m *fakeModel) GetName() string   { return m.name }
func (m *fakeModel) GetMaxTokens() int { return 0 }
func (m *fakeModel) SetTools(t []tools.Tool) error {
	m.setTools = t
	return nil
}

func echoTool(name string) tools.Tool {
	return &tools.BaseTool{
		Name:        name,
		Description: "describes " + name,
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ExecuteFn:   func(json.RawMessage) (string, error) { return "ok", nil },
	}
}

func TestBuildSystemPromptListsTools(t *testing.T) {
	ts := []tools.Tool{echoTool("alpha"), echoTool("beta")}
	for _, name := range []string{"claude-x", "ollama-llama3"} {
		prompt := buildSystemPrompt(name, ts)
		for _, want := range []string{"- alpha: describes alpha", "- beta: describes beta"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s prompt missing %q", name, want)
			}
		}
	}
}

func TestNewAgentRegistersProvidedTools(t *testing.T) {
	m := &fakeModel{name: "fake"}
	ts := []tools.Tool{echoTool("alpha")}
	a, err := NewAgent(m, nil, ts, false, filepath.Join(t.TempDir(), "h.json"), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.setTools) != 1 || m.setTools[0].GetName() != "alpha" {
		t.Fatalf("model tools = %v, want [alpha]", m.setTools)
	}
	if len(a.tools) != 1 {
		t.Fatalf("agent tools = %d, want 1", len(a.tools))
	}
}
