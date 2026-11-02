// ABOUTME: Tests for provider message conversion and tool call extraction.
// ABOUTME: ChatGPT streaming is exercised against a local SSE server.
package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-agent/pkg/tools"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/sashabaranov/go-openai"
)

var toolHistory = []Message{
	{Role: "system", Content: "be helpful"},
	{Role: "user", Content: "hi"},
	{Role: "assistant", Content: "checking", ToolCalls: []ToolCall{
		{ID: "a", Name: "read_file", Input: json.RawMessage(`{"path":"x"}`)},
		{ID: "b", Name: "list_dir", Input: json.RawMessage(`{"path":"."}`)},
	}},
	{Role: "tool", Content: "file body", ToolCallID: "a"},
	{Role: "tool", Content: "dir listing", ToolCallID: "b"},
}

func TestToAnthropicMessagesMapsRoles(t *testing.T) {
	system, msgs := toAnthropicMessages(toolHistory)
	if len(system) != 1 || system[0].Text != "be helpful" {
		t.Fatalf("system = %+v", system)
	}
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3 (user, assistant, merged tool results)", len(msgs))
	}
	if msgs[0].Role != anthropic.MessageParamRoleUser || msgs[1].Role != anthropic.MessageParamRoleAssistant || msgs[2].Role != anthropic.MessageParamRoleUser {
		t.Fatalf("roles = %s,%s,%s", msgs[0].Role, msgs[1].Role, msgs[2].Role)
	}
	if n := len(msgs[1].Content); n != 3 {
		t.Fatalf("assistant blocks = %d, want text + 2 tool_use", n)
	}
	if use := msgs[1].Content[1].OfRequestToolUseBlock; use == nil || use.ID != "a" || use.Name != "read_file" {
		t.Fatalf("tool_use block = %+v", msgs[1].Content[1])
	}
	if n := len(msgs[2].Content); n != 2 {
		t.Fatalf("tool result blocks = %d, want 2", n)
	}
	if r := msgs[2].Content[1].OfRequestToolResultBlock; r == nil || r.ToolUseID != "b" {
		t.Fatalf("second tool_result = %+v", msgs[2].Content[1])
	}
}

func TestToOpenAIMessagesMapsToolCalls(t *testing.T) {
	msgs := toOpenAIMessages(toolHistory)
	if msgs[2].Role != "assistant" || len(msgs[2].ToolCalls) != 2 || msgs[2].ToolCalls[1].Function.Arguments != `{"path":"."}` {
		t.Fatalf("assistant = %+v", msgs[2])
	}
	if msgs[3].Role != "tool" || msgs[3].ToolCallID != "a" {
		t.Fatalf("tool = %+v", msgs[3])
	}
}

func TestChatGPTStreamAssemblesToolCallDeltas(t *testing.T) {
	chunks := []string{
		`{"choices":[{"index":0,"delta":{"content":"let me look"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"pa"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"a.go\"}"}}]}}]}`,
		`{"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":7}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := openai.DefaultConfig("test")
	cfg.BaseURL = srv.URL
	m := &ChatGPTModel{client: openai.NewClientWithConfig(cfg), config: ModelConfig{ModelName: "gpt-test"}}

	var streamed strings.Builder
	resp, err := m.StreamResponse(context.Background(), []Message{{Role: "user", Content: "hi"}}, func(c string) error {
		streamed.WriteString(c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if streamed.String() != "let me look" || resp.Content != "let me look" {
		t.Errorf("content = %q / %q", streamed.String(), resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
	call := resp.ToolCalls[0]
	if call.ID != "call_1" || call.Name != "read_file" || string(call.Input) != `{"path":"a.go"}` {
		t.Errorf("call = %+v input=%s", call, call.Input)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 7 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestParseTextToolCall(t *testing.T) {
	call, ok := parseTextToolCall("sure\n<tool>\n{\"name\": \"read_file\", \"arguments\": {\"path\": \"a\"}}\n</tool>")
	if !ok || call.Name != "read_file" || string(call.Input) != `{"path": "a"}` {
		t.Fatalf("call = %+v ok=%v", call, ok)
	}
	if _, ok := parseTextToolCall("no tools here"); ok {
		t.Fatal("expected no call")
	}
	call, ok = parseTextToolCall("<tool>{broken</tool>")
	if !ok || call.Name != "" {
		t.Fatalf("malformed call = %+v ok=%v", call, ok)
	}
}

func TestClaudeRequestUsesConfigAndToolDescriptions(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"x\",\"usage\":{\"input_tokens\":3,\"output_tokens\":0}}}\n\n")
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n")
		fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n")
		fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	m, err := NewClaudeModel(ModelConfig{APIKey: "key-from-config", ModelName: "claude-test", MaxTokens: 10, Temperature: 0.2})
	if err != nil {
		t.Fatal(err)
	}
	client := anthropic.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL))
	m.client = &client
	if err := m.SetTools([]tools.Tool{tools.NewReadFileTool()}); err != nil {
		t.Fatal(err)
	}

	resp, err := m.StreamResponse(context.Background(), []Message{{Role: "user", Content: "hi"}}, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "hello" || resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 5 {
		t.Errorf("resp = %+v", resp)
	}
	if body["model"] != "claude-test" || body["temperature"] != 0.2 {
		t.Errorf("model/temperature = %v/%v", body["model"], body["temperature"])
	}
	toolsJSON, _ := json.Marshal(body["tools"])
	if !strings.Contains(string(toolsJSON), `"description":"Read the contents`) {
		t.Errorf("tool description missing: %s", toolsJSON)
	}
}

func TestOllamaStreamsBeforeResponseCompletes(t *testing.T) {
	firstSeen := make(chan struct{})
	var reqBody ollamaRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"hel"},"done":false}`)
		w.(http.Flusher).Flush()
		// Hold the response open until the client has handled the first chunk
		select {
		case <-firstSeen:
		case <-time.After(2 * time.Second):
			t.Error("first chunk was not delivered before the response completed")
		}
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"lo","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"a"}}},{"function":{"name":"list_dir","arguments":{"path":"."}}}]},"done":false}`)
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":9,"eval_count":4}`)
	}))
	defer srv.Close()

	m, _ := NewOllamaModel(ModelConfig{ModelName: "llama-test"})
	m.baseURL = srv.URL
	if err := m.SetTools([]tools.Tool{tools.NewReadFileTool()}); err != nil {
		t.Fatal(err)
	}

	chunks := 0
	resp, err := m.StreamResponse(context.Background(), []Message{{Role: "user", Content: "hi"}}, func(string) error {
		chunks++
		if chunks == 1 {
			close(firstSeen)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "hello" {
		t.Errorf("content = %q", resp.Content)
	}
	if len(resp.ToolCalls) != 2 || resp.ToolCalls[1].Name != "list_dir" || resp.ToolCalls[0].ID == resp.ToolCalls[1].ID {
		t.Errorf("tool calls = %+v", resp.ToolCalls)
	}
	if resp.Usage.InputTokens != 9 || resp.Usage.OutputTokens != 4 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if len(reqBody.Tools) != 1 || reqBody.Tools[0].Function.Name != "read_file" {
		t.Errorf("request tools = %+v", reqBody.Tools)
	}
}
