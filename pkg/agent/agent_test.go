// ABOUTME: Tests for the agent loop, system prompt construction, and tool dispatch.
// ABOUTME: Uses a scripted fake model so no network access is required.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-agent/pkg/models"
	"llm-agent/pkg/tools"
)

// fakeModel replays scripted responses and records the history it was sent on each call.
// A nil entry in responses makes that call fail.
type fakeModel struct {
	name      string
	setTools  []tools.Tool
	responses []*models.Response
	seen      [][]models.Message
}

func (m *fakeModel) StreamResponse(ctx context.Context, messages []models.Message, onChunk func(string) error) (*models.Response, error) {
	m.seen = append(m.seen, append([]models.Message(nil), messages...))
	if len(m.responses) == 0 {
		return &models.Response{Content: "done"}, nil
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	if resp == nil {
		return nil, errors.New("api unavailable")
	}
	if resp.Content != "" {
		if err := onChunk(resp.Content); err != nil {
			return nil, err
		}
	}
	return resp, nil
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
		ExecuteFn:   func(in json.RawMessage) (string, error) { return name + " got " + string(in), nil },
	}
}

// inputs returns a getUserInput func that yields each line once, then reports EOF
func inputs(lines ...string) func() (string, bool) {
	return func() (string, bool) {
		if len(lines) == 0 {
			return "", false
		}
		line := lines[0]
		lines = lines[1:]
		return line, true
	}
}

func newTestAgent(t *testing.T, m *fakeModel, lines ...string) *Agent {
	t.Helper()
	a, err := NewAgent(m, inputs(lines...), []tools.Tool{echoTool("alpha"), echoTool("beta")}, false, filepath.Join(t.TempDir(), "h.json"))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func roles(msgs []models.Message) string {
	r := make([]string, len(msgs))
	for i, m := range msgs {
		r[i] = m.Role
	}
	return strings.Join(r, ",")
}

func TestRunChainsToolCallsUntilTextAnswer(t *testing.T) {
	m := &fakeModel{name: "fake", responses: []*models.Response{
		{ToolCalls: []models.ToolCall{{ID: "c1", Name: "alpha", Input: json.RawMessage(`{"x":1}`)}}},
		{Content: "now beta", ToolCalls: []models.ToolCall{{ID: "c2", Name: "beta", Input: json.RawMessage(`{}`)}}},
		{Content: "final answer"},
	}}
	a := newTestAgent(t, m, "hello", "second")
	if err := a.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(m.seen) != 4 {
		t.Fatalf("model called %d times, want 4", len(m.seen))
	}
	// Third call must see both tool results, each answering its own call ID
	third := m.seen[2]
	if got, want := roles(third), "system,user,assistant,tool,assistant,tool"; got != want {
		t.Fatalf("roles = %s, want %s", got, want)
	}
	if third[3].ToolCallID != "c1" || third[3].Content != `alpha got {"x":1}` {
		t.Errorf("first tool result = %+v", third[3])
	}
	if third[5].ToolCallID != "c2" || third[5].Content != "beta got {}" {
		t.Errorf("second tool result = %+v", third[5])
	}
	// The next user turn sees the final answer exactly once, with no duplicated content
	fourth := m.seen[3]
	if got, want := roles(fourth), "system,user,assistant,tool,assistant,tool,assistant,user"; got != want {
		t.Fatalf("roles = %s, want %s", got, want)
	}
	if fourth[6].Content != "final answer" {
		t.Errorf("final assistant content = %q", fourth[6].Content)
	}
}

func TestRunAnswersUnknownAndMalformedToolCalls(t *testing.T) {
	m := &fakeModel{name: "fake", responses: []*models.Response{
		{ToolCalls: []models.ToolCall{
			{ID: "c1", Name: "nope", Input: json.RawMessage(`{}`)},
			{ID: "c2", Name: "", Input: json.RawMessage(`not json`)},
		}},
	}}
	a := newTestAgent(t, m, "hello")
	if err := a.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := m.seen[1]
	if !strings.Contains(second[3].Content, `unknown tool "nope"`) {
		t.Errorf("unknown tool result = %q", second[3].Content)
	}
	if !strings.Contains(second[4].Content, "malformed tool call") {
		t.Errorf("malformed tool result = %q", second[4].Content)
	}
}

func TestRunStopsAfterMaxToolIterations(t *testing.T) {
	var script []*models.Response
	for i := 0; i < maxToolIterations+5; i++ {
		script = append(script, &models.Response{ToolCalls: []models.ToolCall{{ID: "c", Name: "alpha", Input: json.RawMessage(`{}`)}}})
	}
	m := &fakeModel{name: "fake", responses: script}
	a := newTestAgent(t, m, "hello")
	if err := a.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(m.seen) != maxToolIterations {
		t.Fatalf("model called %d times, want %d", len(m.seen), maxToolIterations)
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
	a, err := NewAgent(m, nil, ts, false, filepath.Join(t.TempDir(), "h.json"))
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

func TestRunUsesOneConversationIDPerSession(t *testing.T) {
	m := &fakeModel{name: "fake"}
	path := filepath.Join(t.TempDir(), "h.jsonl")
	a, err := NewAgent(m, inputs("one", "two"), nil, false, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		var rec struct {
			ConversationID string `json:"conversation_id"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		ids[rec.ConversationID] = true
	}
	if len(lines) != 4 || len(ids) != 1 {
		t.Fatalf("%d records with %d conversation IDs, want 4 records sharing 1", len(lines), len(ids))
	}
}

func TestRunReturnsToolErrorsToModel(t *testing.T) {
	failing := &tools.BaseTool{
		Name:        "fails",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		ExecuteFn:   func(json.RawMessage) (string, error) { return "", errors.New("no such file") },
	}
	m := &fakeModel{name: "fake", responses: []*models.Response{
		{ToolCalls: []models.ToolCall{{ID: "c1", Name: "fails", Input: json.RawMessage(`{}`)}}},
	}}
	a, err := NewAgent(m, inputs("hello"), []tools.Tool{failing}, false, filepath.Join(t.TempDir(), "h.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run returned %v, want tool error reported to model", err)
	}
	if got := m.seen[1][3].Content; got != "error: no such file" {
		t.Errorf("tool result = %q", got)
	}
}

func TestRunSurvivesModelErrors(t *testing.T) {
	m := &fakeModel{name: "fake", responses: []*models.Response{
		{ToolCalls: []models.ToolCall{{ID: "c1", Name: "alpha", Input: json.RawMessage(`{}`)}}},
		nil, // fails after a tool call, mid-turn
		{Content: "recovered"},
	}}
	a := newTestAgent(t, m, "first", "second")
	if err := a.Run(context.Background()); err != nil {
		t.Fatalf("Run returned %v, want it to continue after a model error", err)
	}
	if len(m.seen) != 3 {
		t.Fatalf("model called %d times, want 3", len(m.seen))
	}
	// The failed turn is dropped entirely, leaving no unanswered user or tool messages
	if got, want := roles(m.seen[2]), "system,user"; got != want {
		t.Fatalf("roles = %s, want %s", got, want)
	}
	if m.seen[2][1].Content != "second" {
		t.Errorf("user content = %q", m.seen[2][1].Content)
	}
}
