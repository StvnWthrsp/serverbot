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
}
