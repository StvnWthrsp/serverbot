package main

import (
	"bytes"
	"context"
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

func TestAgentRequestRoutesOPToPersistentSessionWithoutOperationAllowlist(t *testing.T) {
	t.Setenv("MSA_OPENCODE_PASSWORD", "synthetic-opencode-password")
	t.Setenv("MSA_RCON_PASSWORD", "synthetic-rcon-password")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "server.properties"), []byte("online-mode=true\n"), 0600)
	os.WriteFile(filepath.Join(dir, "ops.json"), []byte(`[{"name":"Admin","uuid":"12345678-1234-1234-1234-123456789abc","level":4}]`), 0600)
	var completed atomic.Bool
	var prompts atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/info":
			fmt.Fprint(w, `{"version":"2.0.32"}`)
		case "/api/session/active":
			fmt.Fprint(w, `{"data":{}}`)
		case "/api/session/ses_fixture/inbox":
			fmt.Fprint(w, `{"data":[]}`)
		case "/api/session":
			var body struct {
				Permissions []struct{ Effect string }
				Location    struct{ Directory string }
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Permissions) != 1 || body.Permissions[0].Effect != "allow" || body.Location.Directory != dir {
				t.Error("agent did not receive full tools at the Minecraft working directory")
			}
			fmt.Fprint(w, `{"data":{"id":"ses_fixture"}}`)
		case "/api/session/ses_fixture/form":
			fmt.Fprint(w, `{"data":[]}`)
		case "/api/session/ses_fixture/message":
			if !completed.Load() {
				fmt.Fprint(w, `{"data":[]}`)
			} else {
				fmt.Fprint(w, `{"data":[{"id":"msg_idle","type":"idle","outcome":"succeeded"},{"id":"msg_reply","type":"assistant","content":[{"type":"text","text":"Fixture agent completed."}]}]}`)
			}
		case "/api/session/ses_fixture/model", "/api/session/ses_fixture/agent":
			w.WriteHeader(204)
		case "/api/session/ses_fixture/prompt":
			var body struct{ Text string }
			json.NewDecoder(r.Body).Decode(&body)
			if !strings.Contains(body.Text, "Install a compatible mod and restart") || strings.Contains(body.Text, "Return exactly one JSON") {
				t.Error("administration request was reduced to a proposal")
			}
			prompts.Add(1)
			completed.Store(true)
			fmt.Fprint(w, `{"data":{"id":"msg_prompt"}}`)
		default:
			t.Error("unexpected route")
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	path := filepath.Join(dir, "agent.json")
	c := map[string]any{"server_dir": dir, "rcon_address": "127.0.0.1:1", "audit_file": "audit.jsonl", "poll_interval_ms": 1000, "request_timeout_seconds": 5, "planner": "agent", "opencode": map[string]string{"url": s.URL, "provider_id": "fixture", "model_id": "fixture"}}
	b, _ := json.Marshal(c)
	os.WriteFile(path, b, 0600)
	var out bytes.Buffer
	if err := run(t.Context(), []string{"request", "-config", path, "-player", "Admin", "-message", "minecraft-server-agent Install a compatible mod and restart"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Fixture agent completed") || prompts.Load() != 1 {
		t.Fatal("agent response not returned")
	}
	if err := run(t.Context(), []string{"request", "-config", path, "-player", "Stranger", "-message", "Install a compatible mod and restart"}, &out); err == nil || prompts.Load() != 1 {
		t.Fatal("non-OP reached the agent")
	}
}

type replyRecorder struct{ commands []string }

func (r *replyRecorder) Command(_ context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	return "", nil
}

func TestAgentRepliesAreBoundedAndJSONEncoded(t *testing.T) {
	rc := &replyRecorder{}
	text := strings.Repeat("🙂", 900) + "\nstop"
	if err := privateReply(t.Context(), rc, "Admin", text); err != nil {
		t.Fatal(err)
	}
	var joined strings.Builder
	for _, command := range rc.commands {
		if len(command) > 4096 || strings.Contains(command, "\n") {
			t.Fatal("reply escaped the RCON command boundary")
		}
		var payload map[string]string
		if json.Unmarshal([]byte(strings.TrimPrefix(command, "tellraw Admin ")), &payload) != nil {
			t.Fatal("reply was not valid JSON")
		}
		joined.WriteString(strings.TrimPrefix(payload["text"], "[Minecraft Server Agent] "))
	}
	if len(rc.commands) != 3 || joined.String() != text {
		t.Fatal("reply splitting corrupted Unicode or text")
	}
	if err := privateReply(t.Context(), rc, "Admin\nstop", "hello"); err == nil {
		t.Fatal("invalid recipient accepted")
	}
}
