package bot

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/minecraft"
)

var ErrAgentLimit = errors.New("hourly agent request limit reached; try again later")
var ErrAgentInterrupted = errors.New("agent interrupted; inspect the OpenCode session and server state before retrying")
var ErrAgentSessionGone = errors.New("the recorded OpenCode session no longer exists")

type AgentReply struct {
	Text    string
	Waiting bool
}

type AgentRuntime interface {
	Create(context.Context, string) (string, error)
	Run(context.Context, string, string) (AgentReply, error)
	Interrupt(context.Context, string) error
}

type AgentJournal interface {
	Journal
	Session(string) string
	SessionState(string) string
	Sessions() map[string]string
}

// AgentEngine authorizes and audits dispatch; OpenCode owns tool execution.
type AgentEngine struct {
	Auth           Authorizer
	Runtime        AgentRuntime
	Audit          AgentJournal
	Cooldown       time.Duration
	BudgetPath     string
	MaxRunsPerHour int
	OnDispatch     func(context.Context, string, string)
	mu             sync.Mutex
	activeMu       sync.Mutex
	active         map[string]context.CancelFunc
}

func AgentMessage(text string) string {
	text = strings.TrimSpace(text)
	const prefix = "minecraft-server-agent"
	if len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix) {
		text = strings.TrimSpace(strings.TrimLeft(text[len(prefix):], ",:"))
	}
	return text
}

func (e *AgentEngine) Handle(ctx context.Context, req minecraft.Request) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if req.ID == "" || len(req.ID) > 128 || !minecraft.ValidUsername(req.Player) || len(req.Text) > 512 || AgentMessage(req.Text) == "" {
		return "", errors.New("invalid agent request")
	}
	if e.Audit.Seen(req.ID) {
		return "", nil
	}
	uuid, err := e.Auth.Authorize(req.Player)
	if err != nil {
		return "", err
	}
	uuid = strings.ToLower(uuid)
	if e.Audit.SessionState(uuid) != "agent_waiting" && e.Cooldown > 0 && time.Since(e.Audit.LastRequest(uuid)) < e.Cooldown {
		return "", ErrCooldown
	}
	id := e.Audit.Session(uuid)
	r := Record{Time: time.Now().UTC(), ID: req.ID, Player: req.Player, UUID: uuid, SessionID: id, State: "accepted"}
	if err := e.Audit.Append(r); err != nil {
		return "", errors.New("cannot audit request; agent not dispatched")
	}
	if AgentMessage(req.Text) == "/new" {
		if id != "" {
			if err := e.Runtime.Interrupt(ctx, id); err != nil && !errors.Is(err, ErrAgentSessionGone) {
				return "", err
			}
		}
		r.State = "agent_reset"
		if err := e.Audit.Append(r); err != nil {
			return "", errors.New("cannot record conversation reset")
		}
		return "Conversation reset. Your next request starts a new OpenCode session.", nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	e.activeMu.Lock()
	if e.active == nil {
		e.active = make(map[string]context.CancelFunc)
	}
	e.active[uuid] = cancel
	e.activeMu.Unlock()
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				current, err := e.Auth.Authorize(req.Player)
				if err != nil || !strings.EqualFold(current, uuid) {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-watchDone
		e.activeMu.Lock()
		delete(e.active, uuid)
		e.activeMu.Unlock()
	}()
	if err := ReserveAttempt(runCtx, e.BudgetPath, e.MaxRunsPerHour); err != nil {
		if errors.Is(err, ErrInferenceLimit) {
			return "", ErrAgentLimit
		}
		return "", err
	}
	if id == "" {
		id, err = e.Runtime.Create(runCtx, "Minecraft admin: "+req.Player+" ("+uuid+")")
		if err != nil {
			return "", err
		}
		r.SessionID, r.State = id, "session_created"
		if err := e.Audit.Append(r); err != nil {
			return "", errors.New("cannot record session; agent not dispatched")
		}
	}
	if err := runCtx.Err(); err != nil {
		return "", err
	}
	current, err := e.Auth.Authorize(req.Player)
	if err != nil || !strings.EqualFold(current, uuid) {
		return "", errors.New("authorization changed before agent dispatch")
	}
	r.Time, r.State = time.Now().UTC(), "agent_started"
	if err := e.Audit.Append(r); err != nil {
		return "", errors.New("cannot audit dispatch; agent not started")
	}
	if e.OnDispatch != nil {
		e.OnDispatch(runCtx, req.Player, id)
	}
	result, runErr := e.Runtime.Run(runCtx, id, AgentMessage(req.Text))
	interruptedConfirmed := true
	if runCtx.Err() != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		stopErr := e.Runtime.Interrupt(stopCtx, id)
		stopCancel()
		if stopErr != nil && !errors.Is(stopErr, ErrAgentSessionGone) {
			interruptedConfirmed = false
			runErr = errors.New("agent interruption could not be confirmed; inspect the OpenCode session and server state")
		}
	}
	if interruptedConfirmed && errors.Is(runCtx.Err(), context.Canceled) && ctx.Err() == nil {
		runErr = ErrAgentInterrupted
	}
	r.Time, r.State = time.Now().UTC(), "agent_completed"
	if result.Waiting {
		r.State = "agent_waiting"
	}
	if runErr != nil {
		r.State = "agent_failed"
	}
	if err := e.Audit.Append(r); err != nil {
		return "", errors.New("cannot record agent outcome; inspect the OpenCode session and server state")
	}
	return result.Text, runErr
}

func (e *AgentEngine) Stop(ctx context.Context, req minecraft.Request) (string, error) {
	if req.ID == "" || len(req.ID) > 128 || !minecraft.ValidUsername(req.Player) {
		return "", errors.New("invalid stop request")
	}
	uuid, err := e.Auth.Authorize(req.Player)
	if err != nil {
		return "", err
	}
	uuid = strings.ToLower(uuid)
	id := e.Audit.Session(uuid)
	auditErr := e.Audit.Append(Record{Time: time.Now().UTC(), ID: req.ID, Player: req.Player, UUID: uuid, SessionID: id, State: "agent_interrupt_requested"})
	e.activeMu.Lock()
	active := e.active[uuid]
	if active != nil {
		active()
	}
	e.activeMu.Unlock()
	if id != "" {
		if err := e.Runtime.Interrupt(ctx, id); err != nil {
			if errors.Is(err, ErrAgentSessionGone) {
				if err := e.Audit.Append(Record{Time: time.Now().UTC(), ID: req.ID, Player: req.Player, UUID: uuid, State: "agent_reset"}); err != nil {
					return "", err
				}
				return "The old conversation no longer exists. Your next request creates a new one.", nil
			}
			return "", err
		}
	}
	if auditErr != nil {
		return "", errors.New("stop attempted but its audit record failed; inspect the OpenCode session")
	}
	if id == "" && active == nil {
		return "No OpenCode conversation to stop.", nil
	}
	return "Stop requested. Completed or detached actions may still need recovery; inspect the OpenCode session.", nil
}

// Recover interrupts only this bridge's recorded sessions; requests are never replayed automatically.
func (e *AgentEngine) Recover(ctx context.Context) error {
	for uuid, id := range e.Audit.Sessions() {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := e.Runtime.Interrupt(stopCtx, id)
		cancel()
		if err != nil && !errors.Is(err, ErrAgentSessionGone) {
			return errors.New("cannot interrupt recorded OpenCode session; inspect it before restarting the bridge")
		}
		state := "agent_interrupted"
		if errors.Is(err, ErrAgentSessionGone) {
			state = "agent_reset"
		}
		if err := e.Audit.Append(Record{Time: time.Now().UTC(), ID: rand.Text(), UUID: uuid, SessionID: id, State: state}); err != nil {
			return err
		}
	}
	return nil
}
