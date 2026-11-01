package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/config"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/opencode"
)

func TestSetupCreatesValidatedConfigurationWithoutSecrets(t *testing.T) {
	t.Setenv("MSA_RCON_PASSWORD", "synthetic-rcon-secret")
	path := filepath.Join(t.TempDir(), "agent.json")
	var out bytes.Buffer
	if err := setup(t.Context(), strings.NewReader("\n\n\n\n\n\nno\n"), &out, path); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Planner != "commands" || c.Limits.PlayerCooldownSeconds != 10 || c.Limits.QueueCapacity != 8 || c.Limits.InferenceCallsPerHour != 60 || !filepath.IsAbs(c.ServerDir) {
		t.Fatal("incorrect setup defaults")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b)+out.String(), "synthetic-rcon-secret") {
		t.Fatal("setup exposed an environment secret")
	}
	if err := setup(t.Context(), strings.NewReader(""), &out, path); err == nil {
		t.Fatal("existing configuration was overwritten")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(b, after) {
		t.Fatal("existing configuration changed")
	}
}

func TestSetupCancellationInterruptsInputWithoutWriting(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	path := filepath.Join(t.TempDir(), "agent.json")
	done := make(chan error, 1)
	go func() { done <- setup(ctx, r, io.Discard, path) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("setup did not stop on cancellation")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("canceled setup created configuration")
	}
}

func TestSetupRejectsIncompleteOrInvalidAnswersBeforeWriting(t *testing.T) {
	for _, answers := range []string{"", "\nexample.com:25575\n\n\n\n\nno\n", "\n\n\n0\n\n\nno\n", "\n\n\n\n101\n\nno\n", "\n\n\n\n\n0\nno\n", "\n\n\nword\n", "\n\n\n\n\n\nmaybe\n"} {
		path := filepath.Join(t.TempDir(), "agent.json")
		if err := setup(t.Context(), strings.NewReader(answers), &bytes.Buffer{}, path); err == nil {
			t.Fatal("invalid setup accepted")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("invalid setup created configuration")
		}
	}
}

func TestSetupConnectsChosenKeyWithoutInference(t *testing.T) {
	t.Setenv("MSA_OPENCODE_PASSWORD", "synthetic-server-secret")
	t.Setenv("CHOSEN_PROVIDER_KEY", "synthetic-provider-secret")
	connected := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/info":
			w.Write([]byte(`{"version":"2.0.32"}`))
		case "/api/integration":
			w.Write([]byte(`{"data":[]}`))
		case "/api/integration/openrouter/connect/key":
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["key"] != "synthetic-provider-secret" {
				t.Error("incorrect provider key")
			}
			connected = true
			w.WriteHeader(204)
		default:
			t.Error("setup made an unexpected or paid inference request")
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	path := filepath.Join(t.TempDir(), "agent.json")
	answers := strings.Join([]string{"", "", "opencode", s.URL, "openrouter", "publisher/model", "CHOSEN_PROVIDER_KEY", "5", "2", "20", "no", ""}, "\n")
	var out bytes.Buffer
	if err := setup(t.Context(), strings.NewReader(answers), &out, path); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(path)
	if err != nil || !connected || c.OpenCode.ModelID != "publisher/model" {
		t.Fatal("provider setup failed")
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b)+out.String(), "synthetic-") {
		t.Fatal("setup exposed a credential")
	}
}

func TestDoctorReportsAllFailuresAndDefaultsToNoInference(t *testing.T) {
	t.Setenv("MSA_RCON_PASSWORD", "")
	t.Setenv("MSA_OPENCODE_PASSWORD", "")
	c := config.Config{ServerDir: t.TempDir(), Planner: "commands", Limits: &config.Limits{PlayerCooldownSeconds: 10, QueueCapacity: 8, InferenceCallsPerHour: 60}, RequestTimeoutSeconds: 1}
	var out bytes.Buffer
	if err := doctor(t.Context(), c, &out, false); err == nil {
		t.Fatal("missing configuration passed diagnostics")
	}
	for _, want := range []string{"FAIL Minecraft files", "FAIL RCON", "MSA_RCON_PASSWORD", "server_dir"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing diagnostic %q", want)
		}
	}
}

func TestDoctorModelProbeIsOptInAndSharesDurableBudget(t *testing.T) {
	t.Setenv("MSA_RCON_PASSWORD", "")
	t.Setenv("MSA_OPENCODE_PASSWORD", "synthetic-local-password")
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/info" {
			w.Write([]byte(`{"version":"2.0.32"}`))
			return
		}
		if r.URL.Path != "/api/experimental/generate" {
			t.Error("unexpected model route")
			w.WriteHeader(404)
			return
		}
		calls++
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"text": `{"action":"status","target":""}`}})
	}))
	defer s.Close()
	dir := t.TempDir()
	c := config.Config{ServerDir: dir, AuditFile: filepath.Join(dir, "audit.jsonl"), Planner: "opencode", OpenCode: &opencode.Config{URL: s.URL, ProviderID: "test", ModelID: "test"}, Limits: &config.Limits{PlayerCooldownSeconds: 10, QueueCapacity: 8, InferenceCallsPerHour: 1}, RequestTimeoutSeconds: 5}
	var out bytes.Buffer
	doctor(t.Context(), c, &out, false)
	if calls != 0 || !strings.Contains(out.String(), "OK OpenCode runtime") || !strings.Contains(out.String(), "SKIP Provider/model access") {
		t.Fatal("ordinary doctor made a model call or skipped runtime diagnostics")
	}
	doctor(t.Context(), c, &out, true)
	if calls != 1 || !strings.Contains(out.String(), "OK Provider/model access") {
		t.Fatal("explicit model check did not verify access")
	}
	doctor(t.Context(), c, &out, true)
	if calls != 1 || !strings.Contains(out.String(), "hourly inference limit reached") {
		t.Fatal("model diagnostics bypassed inference budget")
	}
}
