package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPinnedRuntimeAgentContract(t *testing.T) {
	binary := os.Getenv("MSA_TEST_OPENCODE")
	if binary == "" {
		t.Skip("set MSA_TEST_OPENCODE to the pinned OpenCode executable")
	}
	root := t.TempDir()
	var calls atomic.Int32
	var webCalls atomic.Int32
	webFixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webCalls.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "fixture-web-ok")
	}))
	defer webFixture.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages json.RawMessage `json:"messages"`
			Tools    []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid provider request")
			w.WriteHeader(400)
			return
		}
		if len(body.Tools) == 0 {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Minecraft administration\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		if strings.Contains(string(body.Messages), "parked-input-must-not-replay") {
			t.Error("cancelled inbox input was replayed to the model")
		}
		step := calls.Add(1)
		var shell, webfetch, question string
		for _, tool := range body.Tools {
			if tool.Function.Name == "shell" {
				shell = tool.Function.Name
			}
			if tool.Function.Name == "webfetch" {
				webfetch = tool.Function.Name
			}
			if tool.Function.Name == "question" {
				question = tool.Function.Name
			}
		}
		if shell == "" || webfetch == "" || question == "" {
			t.Errorf("full agent did not expose shell; tool inventory: %+v", body.Tools)
			w.WriteHeader(400)
			return
		}
		delta := map[string]any{"role": "assistant", "content": "Verified fixture tool execution."}
		finish := "stop"
		if step == 1 {
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "fixture_shell", "type": "function", "function": map[string]string{"name": shell, "arguments": `{"command":"echo agent-tool-ok > agent-tool-marker.txt","description":"Write isolated contract fixture"}`}}}}
			finish = "tool_calls"
		}
		if step == 2 {
			args, _ := json.Marshal(map[string]string{"url": webFixture.URL, "format": "text"})
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "fixture_web", "type": "function", "function": map[string]string{"name": webfetch, "arguments": string(args)}}}}
			finish = "tool_calls"
		}
		if step == 3 || step == 5 {
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprint("fixture_question_", step), "type": "function", "function": map[string]string{"name": question, "arguments": `{"questions":[{"question":"Continue with the fixture?","header":"Continue","options":[{"label":"Yes","description":"Continue"},{"label":"No","description":"Stop"}]}]}`}}}}
			finish = "tool_calls"
		}
		chunk := map[string]any{"id": "agent-fixture", "object": "chat.completion.chunk", "created": 1, "model": "agent-test", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
		w.Header().Set("Content-Type", "text/event-stream")
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", b)
	}))
	defer provider.Close()
	configuration := map[string]any{
		"permissions": []any{map[string]string{"action": "*", "resource": "*", "effect": "allow"}},
		"agents":      map[string]any{"build": map[string]any{"steps": 5}},
		"providers":   map[string]any{"fixture": map[string]any{"package": "@opencode/ai/providers/openai-compatible", "settings": map[string]any{"baseURL": provider.URL + "/v1", "apiKey": "synthetic-test-only"}, "models": map[string]any{"agent-test": map[string]any{"limit": map[string]int{"context": 131072, "output": 2048}, "capabilities": map[string]bool{"toolcall": true}}}}},
	}
	server := startPinnedRuntime(t, binary, root, configuration)
	client, err := New(Config{URL: server.url, ProviderID: "fixture", ModelID: "agent-test"}, "", server.password)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	agent := Agent{Client: client, Directory: root}
	id, err := agent.Create(t.Context(), "Isolated full-tool fixture")
	if err != nil {
		t.Fatal(err)
	}
	result, err := agent.Run(t.Context(), id, "Write the requested isolated marker and verify it.")
	if err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(root, "agent-tool-marker.txt"))
	if err != nil || strings.TrimSpace(string(marker)) != "agent-tool-ok" {
		t.Fatal("the real OpenCode agent did not execute its shell tool in the configured directory")
	}
	if webCalls.Load() == 0 {
		t.Fatal("the real OpenCode agent did not execute its web tool")
	}
	if !result.Waiting || !strings.Contains(result.Text, "Continue with the fixture") {
		t.Fatal("native question was not returned to chat")
	}
	result, err = agent.Run(t.Context(), id, "1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Waiting || !strings.Contains(result.Text, "Verified fixture") || calls.Load() < 4 {
		t.Fatalf("agent loop did not complete: %+v, %d calls", result, calls.Load())
	}
	result, err = agent.Run(t.Context(), id, "Ask another fixture question.")
	if err != nil || !result.Waiting {
		t.Fatalf("persistent conversation did not ask another question: %v", err)
	}
	if err := agent.Interrupt(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	forms, err := agent.forms(t.Context(), id)
	if err != nil || len(forms) != 0 {
		t.Fatal("interrupt left the session's question pending")
	}
	var admitted json.RawMessage
	if err := agent.request(t.Context(), http.MethodPost, "/api/session/"+id+"/prompt", map[string]any{"text": "parked-input-must-not-replay", "resume": false}, &admitted); err != nil {
		t.Fatal(err)
	}
	if err := agent.Interrupt(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	result, err = agent.Run(t.Context(), id, "Make a new fixture request without replaying parked inputs.")
	if err != nil || result.Waiting {
		t.Fatal("new request after interruption failed", err)
	}
}
