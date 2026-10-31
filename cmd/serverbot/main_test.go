package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestVersionAndHelpNeedNoCredentials(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"run", "-h"}} {
		var out bytes.Buffer
		if err := run(t.Context(), args, &out); err != nil || out.Len() == 0 {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func TestCLIRejectsUnknownCommand(t *testing.T) {
	if err := run(t.Context(), []string{"shell"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown command accepted")
	}
}

func TestNaturalLanguagePreviewNeedsNoRCON(t *testing.T) {
	t.Setenv("SERVERBOT_RCON_PASSWORD", "")
	t.Setenv("SERVERBOT_OPENCODE_PASSWORD", "synthetic-test-only")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			w.Write([]byte(`{"version":"2.0.32"}`))
			return
		}
		if r.URL.Path != "/api/experimental/generate" {
			t.Error("unexpected API path")
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"text": `{"action":"whitelist_add","target":"Friend_1"}`}})
	}))
	defer s.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	c := map[string]any{"server_dir": "nonexistent-server", "rcon_address": "127.0.0.1:1", "audit_file": "audit.jsonl", "poll_interval_ms": 1000, "request_timeout_seconds": 5, "planner": "opencode", "opencode": map[string]string{"url": s.URL, "provider_id": "test", "model_id": "test"}}
	b, _ := json.Marshal(c)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(t.Context(), []string{"plan", "-config", path, "-message", "serverbot, please whitelist Friend_1"}, &out); err != nil {
		t.Fatal(err)
	}
	var proposal map[string]string
	if json.Unmarshal(out.Bytes(), &proposal) != nil || proposal["action"] != "whitelist_add" || proposal["target"] != "Friend_1" {
		t.Fatal("invalid preview")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "audit.jsonl")); !os.IsNotExist(err) {
		t.Fatal("preview created an execution journal")
	}
}
