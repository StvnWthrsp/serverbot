package bot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/minecraft"
)

type fakeAgent struct {
	mu          sync.Mutex
	created     int
	dispatched  []string
	interrupted []string
	onCreate    func()
	run         func(context.Context, string, string) (AgentReply, error)
}

func (f *fakeAgent) Create(context.Context, string) (string, error) {
	f.mu.Lock()
	f.created++
	id := fmt.Sprint("ses_fixture", f.created)
	f.mu.Unlock()
	if f.onCreate != nil {
		f.onCreate()
	}
	return id, nil
}
func (f *fakeAgent) Run(ctx context.Context, id, text string) (AgentReply, error) {
	f.mu.Lock()
	f.dispatched = append(f.dispatched, id)
	f.mu.Unlock()
	if f.run != nil {
		return f.run(ctx, id, text)
	}
	return AgentReply{Text: "Verified fixture administration."}, nil
}
func (f *fakeAgent) Interrupt(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.interrupted = append(f.interrupted, id)
	return nil
}

func agentFixture(t *testing.T) (*AgentEngine, *fakeAgent, string) {
	e, dir, _ := fixture(t)
	runtime := &fakeAgent{}
	return &AgentEngine{Auth: e.Auth, Runtime: runtime, Audit: e.Audit.(*FileJournal), BudgetPath: filepath.Join(dir, "agent-runs"), MaxRunsPerHour: 60}, runtime, dir
}
func agentRequest(id, text string) minecraft.Request {
	return minecraft.Request{ID: id, Player: "Admin", Text: "minecraft-server-agent " + text}
}

func TestAgentAuthorizesAndAuditsBeforeUnrestrictedDispatch(t *testing.T) {
	e, runtime, _ := agentFixture(t)
	runtime.run = func(_ context.Context, id, text string) (AgentReply, error) {
		if e.Audit.Session(testUUID) != id || e.Audit.SessionState(testUUID) != "agent_started" {
			t.Fatal("agent started before durable ownership/dispatch records")
		}
		if text != "Install a mod and restart the server" {
			t.Fatal("administration request was restricted or rewritten as a proposal")
		}
		return AgentReply{Text: "fixture"}, nil
	}
	req := agentRequest("denied", "Install a mod and restart the server")
	req.Player = "Stranger"
	if _, err := e.Handle(t.Context(), req); err == nil || runtime.created != 0 {
		t.Fatal("unauthorized request reached OpenCode")
	}
	if _, err := e.Handle(t.Context(), agentRequest("allowed", "Install a mod and restart the server")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Handle(t.Context(), agentRequest("allowed", "Install a mod and restart the server")); err != nil || len(runtime.dispatched) != 1 {
		t.Fatal("duplicate agent request replayed")
	}
}

type failedAgentJournal struct{ *FileJournal }

func (failedAgentJournal) Append(Record) error { return errors.New("disk full") }

type deletedSessionRuntime struct{ *fakeAgent }

func (deletedSessionRuntime) Interrupt(context.Context, string) error { return ErrAgentSessionGone }

func TestDeletedSessionRecoveryClearsOwnershipWithoutReplaying(t *testing.T) {
	e, runtime, _ := agentFixture(t)
	if _, err := e.Handle(t.Context(), agentRequest("first", "Inspect server")); err != nil {
		t.Fatal(err)
	}
	e.Runtime = deletedSessionRuntime{runtime}
	if err := e.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	if e.Audit.Session(testUUID) != "" || len(runtime.dispatched) != 1 {
		t.Fatal("deleted session was retained or work was replayed")
	}
}

func TestAgentAuditFailureAndOPRevocationPreventDispatch(t *testing.T) {
	for _, mode := range []string{"audit", "revocation"} {
		t.Run(mode, func(t *testing.T) {
			e, runtime, dir := agentFixture(t)
			if mode == "audit" {
				e.Audit = failedAgentJournal{e.Audit.(*FileJournal)}
			} else {
				runtime.onCreate = func() { os.WriteFile(filepath.Join(dir, "ops.json"), []byte("[]"), 0600) }
			}
			if _, err := e.Handle(t.Context(), agentRequest("one", "Restart the server")); err == nil || len(runtime.dispatched) != 0 {
				t.Fatal("unaudited or revoked request dispatched")
			}
		})
	}
}

func TestAgentConversationOwnershipPersistenceAndReset(t *testing.T) {
	e, runtime, dir := agentFixture(t)
	for _, id := range []string{"first", "followup"} {
		if _, err := e.Handle(t.Context(), agentRequest(id, "Continue administration")); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.created != 1 || runtime.dispatched[0] != runtime.dispatched[1] {
		t.Fatal("follow-up lost its conversation")
	}
	e.Audit.(*FileJournal).Close()
	j, err := OpenJournal(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	e.Audit = j
	if err := e.Recover(t.Context()); err != nil || len(runtime.interrupted) != 1 || len(runtime.dispatched) != 2 {
		t.Fatal("restart recovery replayed work or failed to interrupt the owned session")
	}
	if _, err := e.Handle(t.Context(), agentRequest("after-restart", "Continue administration")); err != nil {
		t.Fatal(err)
	}
	if runtime.created != 1 {
		t.Fatal("restart lost persisted session ownership")
	}
	if _, err := e.Handle(t.Context(), agentRequest("reset", "/new")); err != nil {
		t.Fatal(err)
	}
	if e.Audit.Session(testUUID) != "" {
		t.Fatal("reset retained the old conversation")
	}
	if _, err := e.Handle(t.Context(), agentRequest("new", "Continue administration")); err != nil || runtime.created != 2 {
		t.Fatal("reset did not create a new session")
	}
}

func TestTwoOPsHaveSeparateConversationsAndStopTargets(t *testing.T) {
	e, runtime, dir := agentFixture(t)
	const otherUUID = "87654321-4321-4321-4321-cba987654321"
	os.WriteFile(filepath.Join(dir, "ops.json"), []byte(`[{"name":"Admin","uuid":"`+testUUID+`","level":4},{"name":"Other","uuid":"`+otherUUID+`","level":4}]`), 0600)
	if _, err := e.Handle(t.Context(), agentRequest("admin", "Inspect the server")); err != nil {
		t.Fatal(err)
	}
	other := agentRequest("other", "Inspect the server")
	other.Player = "Other"
	if _, err := e.Handle(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	if e.Audit.Session(testUUID) == e.Audit.Session(otherUUID) {
		t.Fatal("OP conversations were mixed")
	}
	other.ID = "other-stop"
	if _, err := e.Stop(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	if len(runtime.interrupted) != 1 || runtime.interrupted[0] != e.Audit.Session(otherUUID) {
		t.Fatal("stop crossed OP ownership")
	}
}

func TestAgentBudgetCountsInteractionsAndWaitingRepliesBypassCooldown(t *testing.T) {
	e, runtime, _ := agentFixture(t)
	e.Cooldown = time.Hour
	e.MaxRunsPerHour = 2
	runtime.run = func(context.Context, string, string) (AgentReply, error) {
		return AgentReply{Text: "Question", Waiting: true}, nil
	}
	if _, err := e.Handle(t.Context(), agentRequest("one", "Install a mod")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Handle(t.Context(), agentRequest("answer", "Yes")); err != nil {
		t.Fatal("cooldown blocked a question answer", err)
	}
	if _, err := e.Handle(t.Context(), agentRequest("over-budget", "Yes")); !errors.Is(err, ErrAgentLimit) || len(runtime.dispatched) != 2 {
		t.Fatal("agent interactions exceeded the durable allowance")
	}
}

func TestAgentStopAndRevocationInterruptActiveExecution(t *testing.T) {
	for _, mode := range []string{"stop", "revocation"} {
		t.Run(mode, func(t *testing.T) {
			e, runtime, dir := agentFixture(t)
			started := make(chan struct{})
			runtime.run = func(ctx context.Context, _, _ string) (AgentReply, error) {
				close(started)
				<-ctx.Done()
				return AgentReply{}, ctx.Err()
			}
			done := make(chan error, 1)
			go func() { _, err := e.Handle(t.Context(), agentRequest("run", "Restart the server")); done <- err }()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("agent did not start")
			}
			denied := agentRequest("denied-stop", "/stop")
			denied.Player = "Stranger"
			if _, err := e.Stop(t.Context(), denied); err == nil {
				t.Fatal("non-OP could stop a session")
			}
			if mode == "stop" {
				if _, err := e.Stop(t.Context(), agentRequest("stop", "/stop")); err != nil {
					t.Fatal(err)
				}
			} else {
				os.WriteFile(filepath.Join(dir, "ops.json"), []byte("[]"), 0600)
			}
			select {
			case err := <-done:
				if !errors.Is(err, ErrAgentInterrupted) {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("agent did not interrupt")
			}
			runtime.mu.Lock()
			defer runtime.mu.Unlock()
			if len(runtime.interrupted) == 0 || !strings.HasPrefix(runtime.interrupted[0], "ses_fixture") {
				t.Fatal("interruption did not target the owned session")
			}
		})
	}
}
