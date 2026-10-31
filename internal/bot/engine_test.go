package bot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/StvnWthrsp/serverbot/internal/minecraft"
)

const testUUID = "12345678-1234-1234-1234-123456789abc"

type fakeCommander struct {
	commands     []string
	verification string
}

func (f *fakeCommander) Command(_ context.Context, c string) (string, error) {
	f.commands = append(f.commands, c)
	if c == "whitelist list" {
		return f.verification, nil
	}
	return "OK", nil
}

type plannerFunc func(context.Context, string) (Plan, error)

func (p plannerFunc) Plan(ctx context.Context, text string) (Plan, error) { return p(ctx, text) }

type badJournal struct{}

func (badJournal) Seen(string) bool    { return false }
func (badJournal) Append(Record) error { return errors.New("disk full") }

func fixture(t *testing.T) (*Engine, string, *fakeCommander) {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "server.properties"), []byte("online-mode=true\n"), 0600)
	os.WriteFile(filepath.Join(dir, "ops.json"), []byte(`[{"name":"Admin","uuid":"`+testUUID+`","level":4}]`), 0600)
	j, err := OpenJournal(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	rc := &fakeCommander{verification: "There are 1 whitelisted players: Friend_1"}
	return &Engine{Auth: minecraft.Authorizer{Dir: dir}, Planner: CommandPlanner{}, RCON: rc, Audit: j}, dir, rc
}

func TestAuthorizedChatWhitelistsVerifiesAndDeduplicates(t *testing.T) {
	e, _, rc := fixture(t)
	req, ok := minecraft.ParseChat("[12Jun2026 08:58:05.596] [Server thread/INFO] [net.minecraft.server.MinecraftServer/]: <Admin> serverbot whitelist Friend_1")
	if !ok {
		t.Fatal("fixture chat was not parsed")
	}
	got, err := e.Handle(t.Context(), req)
	if err != nil || got != "Verified: Friend_1 is whitelisted." {
		t.Fatalf("%q %v", got, err)
	}
	if got, err = e.Handle(t.Context(), req); err != nil || got != "" {
		t.Fatalf("duplicate: %q %v", got, err)
	}
	if !reflect.DeepEqual(rc.commands, []string{"whitelist add Friend_1", "whitelist list"}) {
		t.Fatal(rc.commands)
	}
}

func TestUntrustedPlayerNeverReachesPlanner(t *testing.T) {
	e, _, rc := fixture(t)
	e.Planner = plannerFunc(func(context.Context, string) (Plan, error) {
		t.Fatal("unauthorized chat reached planner")
		return Plan{}, nil
	})
	_, err := e.Handle(t.Context(), minecraft.Request{ID: "one", Player: "Stranger", Text: "serverbot whitelist Friend_1"})
	if err == nil || len(rc.commands) != 0 {
		t.Fatal("unauthorized request executed")
	}
}

func TestAuthorizationFailsClosed(t *testing.T) {
	for _, mode := range []string{"missing-ops", "broken-ops", "trailing-ops", "offline", "missing-properties"} {
		t.Run(mode, func(t *testing.T) {
			e, dir, rc := fixture(t)
			switch mode {
			case "missing-ops":
				os.Remove(filepath.Join(dir, "ops.json"))
			case "broken-ops":
				os.WriteFile(filepath.Join(dir, "ops.json"), []byte("{"), 0600)
			case "trailing-ops":
				os.WriteFile(filepath.Join(dir, "ops.json"), []byte(`[{"name":"Admin","uuid":"`+testUUID+`","level":4}] {}`), 0600)
			case "offline":
				os.WriteFile(filepath.Join(dir, "server.properties"), []byte("online-mode=false"), 0600)
			case "missing-properties":
				os.Remove(filepath.Join(dir, "server.properties"))
			}
			_, err := e.Handle(t.Context(), minecraft.Request{ID: "one", Player: "Admin", Text: "serverbot status"})
			if err == nil || len(rc.commands) != 0 {
				t.Fatal("failed open")
			}
		})
	}
}

func TestRevocationDuringPlanning(t *testing.T) {
	e, dir, rc := fixture(t)
	e.Planner = plannerFunc(func(context.Context, string) (Plan, error) {
		os.WriteFile(filepath.Join(dir, "ops.json"), []byte("[]"), 0600)
		return Plan{Action: "status"}, nil
	})
	if _, err := e.Handle(t.Context(), minecraft.Request{ID: "one", Player: "Admin", Text: "serverbot status"}); err == nil || len(rc.commands) != 0 {
		t.Fatal("revoked authorization used")
	}
}

func TestPlannerCannotEscapeOperationBoundary(t *testing.T) {
	for _, plan := range []Plan{{Action: "shell", Target: "whoami"}, {Action: "whitelist_add", Target: "Friend\nstop"}, {Action: "whitelist_add", Target: "../../escape"}, {Action: "status", Target: "stop"}} {
		t.Run(plan.Action+plan.Target, func(t *testing.T) {
			e, _, rc := fixture(t)
			e.Planner = plannerFunc(func(context.Context, string) (Plan, error) { return plan, nil })
			_, err := e.Handle(t.Context(), minecraft.Request{ID: "one", Player: "Admin", Text: "untrusted model input"})
			if err == nil || len(rc.commands) != 0 {
				t.Fatal("unsafe plan executed")
			}
		})
	}
}

func TestAuditFailurePreventsExecution(t *testing.T) {
	e, _, rc := fixture(t)
	e.Audit = badJournal{}
	_, err := e.Handle(t.Context(), minecraft.Request{ID: "one", Player: "Admin", Text: "serverbot status"})
	if err == nil || len(rc.commands) != 0 {
		t.Fatal("executed without audit")
	}
}

func TestWhitelistMustBeVerified(t *testing.T) {
	e, _, rc := fixture(t)
	rc.verification = "There are 1 whitelisted players: OtherFriend"
	_, err := e.Handle(t.Context(), minecraft.Request{ID: "one", Player: "Admin", Text: "serverbot whitelist Friend_1"})
	if err == nil {
		t.Fatal("unverified mutation reported success")
	}
}

func TestJournalRecoveryAndExclusiveOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	j, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := OpenJournal(path); err == nil {
		other.Close()
		t.Fatal("second owner accepted")
	}
	if err := j.Append(Record{ID: "incomplete-job", State: "started"}); err != nil {
		t.Fatal(err)
	}
	j.Close()
	j, err = OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if !j.Seen("incomplete-job") {
		t.Fatal("started job would be replayed")
	}
	j.Close()
	os.WriteFile(path, []byte("{partial"), 0600)
	if j, err := OpenJournal(path); err == nil {
		j.Close()
		t.Fatal("corrupt journal accepted")
	}
}
