package bot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/minecraft"
)

func TestInferenceBudgetPersistsFailuresAndExpiresOldReservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inference.jsonl")
	j, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(Record{ID: "old", State: "inference_started", Time: time.Now().Add(-2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	j.Close()
	calls := 0
	planner := plannerFunc(func(context.Context, string) (Plan, error) {
		calls++
		return Plan{}, errors.New("provider unavailable")
	})
	p := BudgetPlanner{Planner: planner, Path: path, MaxCallsPerHour: 1}
	if _, err := p.Plan(t.Context(), "status"); err == nil || errors.Is(err, ErrInferenceLimit) {
		t.Fatal("first attempt was not sent")
	}
	p = BudgetPlanner{Planner: planner, Path: path, MaxCallsPerHour: 1}
	if _, err := p.Plan(t.Context(), "status"); !errors.Is(err, ErrInferenceLimit) || calls != 1 {
		t.Fatal("recreated planner reset the durable allowance")
	}
}

func TestInferenceBudgetFailsClosedForCanceledCorruptAndLockedLedger(t *testing.T) {
	for _, mode := range []string{"canceled", "corrupt", "locked", "missing-time"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "inference.jsonl")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch mode {
			case "canceled":
				cancel()
			case "corrupt":
				os.WriteFile(path, []byte("{broken"), 0600)
			case "locked":
				os.Mkdir(path+".lock", 0700)
			case "missing-time":
				os.WriteFile(path, []byte("{\"id\":\"bad\",\"state\":\"inference_started\"}\n"), 0600)
			}
			p := BudgetPlanner{Planner: plannerFunc(func(context.Context, string) (Plan, error) { t.Fatal("unreserved inference call"); return Plan{}, nil }), Path: path, MaxCallsPerHour: 1}
			if _, err := p.Plan(ctx, "status"); err == nil {
				t.Fatal("failed open")
			}
			if mode == "canceled" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("canceled request reserved inference")
				}
			}
		})
	}
}

func TestConcurrentInferenceCannotExceedAllowance(t *testing.T) {
	var calls atomic.Int32
	p := BudgetPlanner{Planner: plannerFunc(func(context.Context, string) (Plan, error) { calls.Add(1); return Plan{Action: "status"}, nil }), Path: filepath.Join(t.TempDir(), "calls.jsonl"), MaxCallsPerHour: 1}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { p.Plan(t.Context(), "status") })
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("got %d calls with an allowance of one", calls.Load())
	}
}

func TestCooldownPersistsAfterFailedPlanningAndUsesUUID(t *testing.T) {
	e, dir, rc := fixture(t)
	e.Cooldown = time.Minute
	calls := 0
	e.Planner = plannerFunc(func(context.Context, string) (Plan, error) { calls++; return Plan{}, errors.New("refused") })
	if _, err := e.Handle(t.Context(), minecraft.Request{ID: "first", Player: "Admin", Text: "anything"}); err == nil {
		t.Fatal("expected refusal")
	}
	e.Audit.(*FileJournal).Close()
	j, err := OpenJournal(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	e.Audit = j
	if _, err := e.Handle(t.Context(), minecraft.Request{ID: "second", Player: "admin", Text: "anything"}); !errors.Is(err, ErrCooldown) || calls != 1 || len(rc.commands) != 0 {
		t.Fatal("cooldown reset or bypassed by player casing")
	}
}

type queueAuth struct{}

func (queueAuth) Authorize(player string) (string, error) {
	if player == "Stranger" {
		return "", errors.New("not an OP")
	}
	return player, nil
}

func TestQueueBoundsPendingRequestsAndCancelsWithoutDraining(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	var calls atomic.Int32
	q := NewQueue(1, time.Minute, queueAuth{}, func(ctx context.Context, _ minecraft.Request) {
		calls.Add(1)
		close(started)
		<-ctx.Done()
	})
	done := make(chan struct{})
	go func() { defer close(done); q.Run(ctx) }()
	req := func(id, player string) minecraft.Request {
		return minecraft.Request{ID: id, Player: player, Text: "status"}
	}
	if err := q.Submit(ctx, req("unauthorized", "Stranger")); err == nil {
		t.Fatal("unauthorized player queued")
	}
	if err := q.Submit(ctx, req("first", "Admin")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	if err := q.Submit(ctx, req("repeat", "Admin")); !errors.Is(err, ErrPlayerBusy) {
		t.Fatal("player occupied a second slot")
	}
	if err := q.Submit(ctx, req("second", "Other")); err != nil {
		t.Fatal(err)
	}
	if err := q.Submit(ctx, req("third", "Third")); !errors.Is(err, ErrQueueFull) {
		t.Fatal("queue capacity exceeded")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	if calls.Load() != 1 {
		t.Fatal("shutdown drained pending requests")
	}
	if err := q.Submit(ctx, req("after", "After")); !errors.Is(err, context.Canceled) {
		t.Fatal("accepted after shutdown")
	}
}

func TestQueueExpiresWaitingWorkBeforeCallingHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var calls atomic.Int32
	handled := make(chan struct{})
	q := NewQueue(2, time.Second, queueAuth{}, func(_ context.Context, req minecraft.Request) {
		if req.ID == "expired" {
			t.Error("expired work reached handler")
		}
		calls.Add(1)
		close(handled)
	})
	q.pending <- queuedRequest{request: minecraft.Request{ID: "expired", Player: "Admin"}, uuid: "admin", deadline: time.Now().Add(-time.Second)}
	if err := q.Submit(ctx, minecraft.Request{ID: "live", Player: "Other", Text: "status"}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); q.Run(ctx) }()
	select {
	case <-handled:
	case <-time.After(time.Second):
		t.Fatal("live request did not run")
	}
	cancel()
	<-done
	if calls.Load() != 1 {
		t.Fatal("unexpected handler count")
	}
}
