package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/bot"
)

func TestAgentPromptFailureAndDeadlineRequestInterruptionWithoutLeakingBodies(t *testing.T) {
	for _, mode := range []string{"failed-prompt", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			var interrupted atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/info":
					fmt.Fprintf(w, `{"version":%q}`, Version)
				case "/api/session/active":
					fmt.Fprint(w, `{"data":{}}`)
				case "/api/session/ses_fixture/inbox":
					fmt.Fprint(w, `{"data":[]}`)
				case "/api/experimental/session/ses_fixture/wait":
					w.WriteHeader(204)
				case "/api/session/ses_fixture/message", "/api/session/ses_fixture/form":
					fmt.Fprint(w, `{"data":[]}`)
				case "/api/session/ses_fixture/model", "/api/session/ses_fixture/agent":
					w.WriteHeader(204)
				case "/api/session/ses_fixture/prompt":
					if mode == "failed-prompt" {
						w.WriteHeader(503)
						fmt.Fprint(w, "private-provider-key-and-error")
					} else {
						fmt.Fprint(w, `{"data":{"id":"msg_fixture"}}`)
					}
				case "/api/session/ses_fixture/interrupt":
					interrupted.Add(1)
					fmt.Fprint(w, `{"interrupted":true}`)
				default:
					t.Error("unexpected agent route")
					w.WriteHeader(404)
				}
			}))
			defer s.Close()
			client, err := New(Config{URL: s.URL, ProviderID: "fixture", ModelID: "fixture"}, "", "synthetic-password")
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			a := Agent{Client: client, Directory: t.TempDir()}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			_, err = a.Run(ctx, "ses_fixture", "Restart the fixture")
			if err == nil || strings.Contains(err.Error(), "private-provider") || strings.Contains(err.Error(), "no action taken") || interrupted.Load() != 1 {
				t.Fatalf("unsafe failure or missing interruption: %v", err)
			}
			if mode == "deadline" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("deadline identity was lost")
			}
		})
	}
}

func TestAgentRejectsUntrustedSessionPaths(t *testing.T) {
	for _, id := range []string{"../session", "ses_fixture/../../other", "", strings.Repeat("x", 200)} {
		a := Agent{}
		if _, err := a.Run(t.Context(), id, "anything"); err == nil {
			t.Fatal("invalid session accepted")
		}
		if err := a.Interrupt(t.Context(), id); err == nil {
			t.Fatal("invalid interruption target accepted")
		}
	}
}

func TestDeletedSessionIsDistinguishedFromUnconfirmedInterruption(t *testing.T) {
	for _, status := range []int{404, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/info" {
					fmt.Fprintf(w, `{"version":%q}`, Version)
					return
				}
				w.WriteHeader(status)
				fmt.Fprint(w, "private-runtime-body")
			}))
			defer s.Close()
			client, err := New(Config{URL: s.URL, ProviderID: "fixture", ModelID: "fixture"}, "", "synthetic-password")
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			err = (Agent{Client: client}).Interrupt(t.Context(), "ses_missing")
			if (status == 404) != errors.Is(err, bot.ErrAgentSessionGone) || err == nil || strings.Contains(err.Error(), "private-runtime-body") || strings.Contains(err.Error(), "no action taken") {
				t.Fatal("incorrect interruption outcome", err)
			}
		})
	}
}

func TestAgentNativeQuestionAnswerFormats(t *testing.T) {
	var form agentForm
	if json.Unmarshal([]byte(`{"id":"frm_fixture","title":"Choose","fields":[{"key":"q0","type":"string","options":[{"value":"Yes","label":"Yes"},{"value":"No","label":"No"}]}]}`), &form) != nil {
		t.Fatal("bad fixture")
	}
	answer, err := formAnswer(form, "2")
	if err != nil || answer["q0"] != "No" {
		t.Fatal("option number not mapped to native value")
	}
	form.Fields[0].Type = "multiselect"
	answer, err = formAnswer(form, "1,2")
	values, _ := answer["q0"].([]string)
	if err != nil || len(values) != 2 || values[0] != "Yes" || values[1] != "No" {
		t.Fatal("multiple selections not mapped")
	}
	form.Fields = append(form.Fields, form.Fields[0])
	form.Fields[1].Key = "q1"
	if _, err := formAnswer(form, `{"q0":["Yes"],"q1":["No"]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := formAnswer(form, "invalid"); err == nil {
		t.Fatal("multiple-field answer accepted without structured values")
	}
}
