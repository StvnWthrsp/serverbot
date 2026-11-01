package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/bot"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/minecraft"
)

func fixture(t *testing.T, handler http.HandlerFunc) *Planner {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "opencode" || p != "synthetic-test-only" {
			t.Error("missing expected Basic authentication")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/info" {
			json.NewEncoder(w).Encode(map[string]string{"version": Version})
			return
		}
		if r.URL.Path != "/api/experimental/generate" || r.Method != "POST" {
			t.Error("unexpected route; must never use session/agent routes")
			w.WriteHeader(404)
			return
		}
		var req map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&req) != nil || len(req) != 2 {
			t.Error("invalid stateless request")
			return
		}
		if _, ok := req["prompt"]; !ok {
			t.Error("prompt missing")
		}
		var model struct{ ID, ProviderID string }
		json.Unmarshal(req["model"], &model)
		if model.ID != "test-model" || model.ProviderID != "test-provider" {
			t.Error("explicit model selection missing")
		}
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	p, err := New(Config{URL: s.URL, ProviderID: "test-provider", ModelID: "test-model"}, "", "synthetic-test-only")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func reply(w http.ResponseWriter, text string) {
	json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"text": text}})
}

func TestStatelessNaturalLanguageProposal(t *testing.T) {
	p := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, `{"action":"whitelist_add","target":"Friend_1"}`)
	})
	plan, err := p.Plan(t.Context(), "minecraft-server-agent, please whitelist Friend_1")
	if err != nil || plan != (bot.Plan{Action: "whitelist_add", Target: "Friend_1"}) {
		t.Fatalf("%+v %v", plan, err)
	}
}

func TestRejectsUnsafeOrMalformedModelOutput(t *testing.T) {
	for _, text := range []string{
		`{"action":"shell","target":"whoami"}`, `{"action":"whitelist_add","target":"Friend\nstop"}`,
		`{"action":"whitelist_add","target":"../escape"}`, `{"action":"status","target":"Friend"}`,
		`{"action":"status"}`, `{"Action":"status","target":""}`, `{"action":"status","target":null}`,
		`{"action":"status","action":"whitelist_add","target":"Friend_1"}`,
		`{"action":"status","target":"","command":"stop"}`, `{"action":"status","target":""} {}`,
		"```json\n{\"action\":\"status\",\"target\":\"\"}\n```", `null`, strings.Repeat("x", 1025),
		`{"action":"refuse","target":""}`,
	} {
		t.Run(fmt.Sprintf("%x", text[:min(len(text), 12)]), func(t *testing.T) {
			p := fixture(t, func(w http.ResponseWriter, r *http.Request) { reply(w, text) })
			if _, err := p.Plan(t.Context(), "minecraft-server-agent status"); err == nil {
				t.Fatal("unsafe output accepted")
			}
		})
	}
}

func TestVersionMismatchNeverGenerates(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `{"version":"2.0.999"}`) }))
	defer s.Close()
	p, _ := New(Config{URL: s.URL, ProviderID: "test", ModelID: "test"}, "", "fake")
	defer p.Close()
	if _, err := p.Plan(t.Context(), "minecraft-server-agent status"); err == nil || calls.Load() != 1 {
		t.Fatal("unsupported runtime used for generation")
	}
}

func TestFailuresAndOversizedResponsesDoNotLeakBodies(t *testing.T) {
	for _, mode := range []string{"auth", "unavailable", "redirect", "bad-json", "extra-json", "large", "missing-text"} {
		t.Run(mode, func(t *testing.T) {
			p := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "auth":
					w.WriteHeader(401)
					fmt.Fprint(w, "private-provider-error")
				case "unavailable":
					w.WriteHeader(503)
					fmt.Fprint(w, "private-provider-error")
				case "redirect":
					http.Redirect(w, r, "http://127.0.0.1:1/should-not-follow", 302)
				case "bad-json":
					fmt.Fprint(w, "private-provider-error")
				case "extra-json":
					fmt.Fprint(w, `{"data":{"text":"{}"}} {}`)
				case "large":
					fmt.Fprint(w, strings.Repeat("x", maxResponse+1))
				case "missing-text":
					fmt.Fprint(w, `{"data":{}}`)
				}
			})
			_, err := p.Plan(t.Context(), "minecraft-server-agent status")
			if err == nil || strings.Contains(err.Error(), "private-provider-error") || strings.Contains(err.Error(), "synthetic-test-only") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}

func TestTimeoutCancelsGeneration(t *testing.T) {
	p := fixture(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	_, err := p.Plan(ctx, "minecraft-server-agent please whitelist Friend_1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lost: %v", err)
	}
}

func TestConfigRejectsRemoteDestinationsAndCredentials(t *testing.T) {
	for _, address := range []string{"http://example.com:4096", "http://localhost:4096", "http://127.0.0.1", "http://user:pass@127.0.0.1:4096", "http://127.0.0.1:4096/path", "http://127.0.0.1:4096/?key=secret", "http://127.0.0.1:4096/#fragment", "file:///tmp/test"} {
		if (Config{URL: address, ProviderID: "test", ModelID: "test"}).Validate() == nil {
			t.Errorf("unsafe URL accepted: %s", address)
		}
	}
}

type commander struct{ calls []string }

func (c *commander) Command(_ context.Context, s string) (string, error) {
	c.calls = append(c.calls, s)
	if s == "whitelist list" {
		return "There are 1 whitelisted players: Friend_1", nil
	}
	return "OK", nil
}

func TestHTTPPlannerThroughAuthorizedEngine(t *testing.T) {
	for _, mode := range []string{"success", "non-op", "revoked", "injection", "invented-target", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			ops := filepath.Join(dir, "ops.json")
			os.WriteFile(filepath.Join(dir, "server.properties"), []byte("online-mode=true\n"), 0600)
			os.WriteFile(ops, []byte(`[{"name":"Admin","uuid":"12345678-1234-1234-1234-123456789abc","level":4}]`), 0600)
			var modelCalls atomic.Int32
			p := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				modelCalls.Add(1)
				if mode == "timeout" {
					<-r.Context().Done()
					return
				}
				if mode == "revoked" {
					os.WriteFile(ops, []byte("[]"), 0600)
				}
				if mode == "injection" {
					reply(w, `{"action":"shell","target":"stop"}`)
					return
				}
				reply(w, `{"action":"whitelist_add","target":"Friend_1"}`)
			})
			journal, err := bot.OpenJournal(filepath.Join(dir, "audit.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer journal.Close()
			rc := &commander{}
			engine := bot.Engine{Auth: minecraft.Authorizer{Dir: dir}, Planner: p, RCON: rc, Audit: journal}
			player := "Admin"
			if mode == "non-op" {
				player = "Stranger"
			}
			text := "minecraft-server-agent please whitelist Friend_1"
			if mode == "invented-target" {
				text = "minecraft-server-agent whitelist somebody"
			}
			ctx := t.Context()
			if mode == "timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 40*time.Millisecond)
				defer cancel()
			}
			_, err = engine.Handle(ctx, minecraft.Request{ID: "one", Player: player, Text: text})
			if mode == "success" {
				if err != nil || len(rc.calls) != 2 {
					t.Fatalf("workflow failed: %v %+v", err, rc.calls)
				}
			} else if err == nil || len(rc.calls) != 0 {
				t.Fatal("unsafe request executed")
			}
			if mode == "non-op" && modelCalls.Load() != 0 {
				t.Fatal("non-OP input reached model")
			}
		})
	}
}

func FuzzParsePlan(f *testing.F) {
	f.Add(`{"action":"status","target":""}`)
	f.Add(`{"action":"whitelist_add","target":"Friend_1"}`)
	f.Fuzz(func(t *testing.T, text string) {
		p, err := parsePlan(text)
		if err == nil && bot.ValidatePlan(p) != nil {
			t.Fatal("invalid plan accepted")
		}
	})
}
