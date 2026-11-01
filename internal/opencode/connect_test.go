package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConnectKeyChecksVersionBeforeSendingCredential(t *testing.T) {
	for _, version := range []string{Version, "unsupported"} {
		t.Run(version, func(t *testing.T) {
			connected := false
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, password, ok := r.BasicAuth()
				if !ok || user != "opencode" || password != "local-test-password" {
					t.Error("missing local server authentication")
				}
				if r.URL.Path == "/api/info" {
					json.NewEncoder(w).Encode(map[string]string{"version": version})
					return
				}
				if r.URL.Path == "/api/integration" {
					fmt.Fprint(w, `{"data":[]}`)
					return
				}
				if r.Method != "POST" || r.URL.Path != "/api/integration/another-provider/connect/key" {
					t.Error("unexpected credential destination")
				}
				var body map[string]string
				if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["key"] != "synthetic-provider-key" {
					t.Error("invalid credential payload")
				}
				connected = true
				w.WriteHeader(http.StatusNoContent)
			}))
			defer s.Close()
			p, err := New(Config{URL: s.URL, ProviderID: "another-provider", ModelID: "unused"}, "", "local-test-password")
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			err = p.ConnectKey(t.Context(), "synthetic-provider-key")
			if (err == nil) != (version == Version) || connected != (version == Version) {
				t.Fatal("credential was not gated on the supported runtime")
			}
		})
	}
}

func TestConnectKeyErrorsDoNotExposeCredentials(t *testing.T) {
	for _, status := range []int{200, 401, 404, 500, 307} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/info" {
					fmt.Fprintf(w, `{"version":%q}`, Version)
					return
				}
				if r.URL.Path == "/api/integration" {
					fmt.Fprint(w, `{"data":[]}`)
					return
				}
				w.Header().Set("Location", "https://example.com/credential")
				w.WriteHeader(status)
				fmt.Fprint(w, "synthetic-provider-key")
			}))
			defer s.Close()
			p, err := New(Config{URL: s.URL, ProviderID: "test", ModelID: "unused"}, "", "synthetic-server-password")
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			err = p.ConnectKey(t.Context(), "synthetic-provider-key")
			if err == nil || strings.Contains(err.Error(), "synthetic-") {
				t.Fatal("credential error was accepted or leaked secrets")
			}
		})
	}
}
