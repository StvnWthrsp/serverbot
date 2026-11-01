package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRejectsRemoteRCONUnknownFieldsAndMultipleDocuments(t *testing.T) {
	base := `{"server_dir":"server","rcon_address":"127.0.0.1:25575","audit_file":"audit.jsonl","poll_interval_ms":1000,"request_timeout_seconds":15}`
	for _, data := range []string{strings.Replace(base, "127.0.0.1", "0.0.0.0", 1), strings.Replace(base, "127.0.0.1", "example.com", 1), strings.Replace(base, `"server_dir"`, `"unknown"`, 1), base + " {}"} {
		path := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(path, []byte(data), 0600)
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(base), 0600)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.AuditFile != filepath.Join(filepath.Dir(path), "audit.jsonl") {
		t.Fatal("relative paths not anchored to config")
	}
	if c.Limits == nil || c.Limits.PlayerCooldownSeconds != 10 || c.Limits.QueueCapacity != 8 || c.Limits.InferenceCallsPerHour != 60 {
		t.Fatal("missing limits did not receive conservative defaults")
	}
}

func TestExplicitLimitsCannotDisableProtection(t *testing.T) {
	for _, limits := range []Limits{{0, 8, 60}, {-1, 8, 60}, {3601, 8, 60}, {10, 0, 60}, {10, 101, 60}, {10, 8, 0}, {10, 8, 10001}} {
		c := Config{ServerDir: "server", RCONAddress: "127.0.0.1:25575", AuditFile: "audit.jsonl", PollIntervalMS: 1000, RequestTimeoutSeconds: 15, Limits: &limits}
		if err := c.Validate(); err == nil {
			t.Fatalf("accepted unsafe limits: %+v", limits)
		}
	}
}

func TestPlannerConfiguration(t *testing.T) {
	base := `{"server_dir":"server","rcon_address":"127.0.0.1:25575","audit_file":"audit.jsonl","poll_interval_ms":1000,"request_timeout_seconds":45`
	valid := `,"planner":"opencode","opencode":{"url":"http://127.0.0.1:4096","provider_id":"test","model_id":"test"}}`
	for _, suffix := range []string{`,"planner":"unknown"}`, `,"planner":"opencode"}`, strings.Replace(valid, `"opencode","opencode"`, `"commands","opencode"`, 1), strings.Replace(valid, `"model_id":"test"`, `"model_id":"test","password":"not-a-real-secret"`, 1)} {
		path := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(path, []byte(base+suffix), 0600)
		if _, err := Load(path); err == nil {
			t.Fatal("invalid planner configuration accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(base+valid), 0600)
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}
