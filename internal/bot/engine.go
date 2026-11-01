package bot

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/minecraft"
)

type Plan struct {
	Action string `json:"action"`
	Target string `json:"target"`
}
type Planner interface {
	Plan(context.Context, string) (Plan, error)
}
type Authorizer interface{ Authorize(string) (string, error) }
type Commander interface {
	Command(context.Context, string) (string, error)
}
type Journal interface {
	Seen(string) bool
	LastRequest(string) time.Time
	Append(Record) error
}
type Record struct {
	Time   time.Time `json:"time"`
	ID     string    `json:"id"`
	Player string    `json:"player"`
	UUID   string    `json:"uuid,omitempty"`
	Action string    `json:"action,omitempty"`
	Target string    `json:"target,omitempty"`
	State  string    `json:"state"`
}

type Engine struct {
	Auth     Authorizer
	Planner  Planner
	RCON     Commander
	Audit    Journal
	Cooldown time.Duration
	mu       sync.Mutex
}

func (e *Engine) Handle(ctx context.Context, req minecraft.Request) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if req.ID == "" || len(req.ID) > 128 || !minecraft.ValidUsername(req.Player) || len(req.Text) > 512 {
		return "", errors.New("invalid request")
	}
	if e.Audit.Seen(req.ID) {
		return "", nil
	}
	uuid, err := e.Auth.Authorize(req.Player)
	if err != nil {
		return "", err
	}
	if e.Cooldown > 0 && time.Since(e.Audit.LastRequest(uuid)) < e.Cooldown {
		return "", ErrCooldown
	}
	if err := e.Audit.Append(Record{Time: time.Now().UTC(), ID: req.ID, Player: req.Player, UUID: uuid, State: "accepted"}); err != nil {
		return "", errors.New("cannot record request; operation refused")
	}
	plan, err := e.Planner.Plan(ctx, req.Text)
	if err != nil {
		return "", err
	}
	if err := ValidatePlan(plan); err != nil {
		return "", err
	}
	if plan.Action == "whitelist_add" && !regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])`+regexp.QuoteMeta(plan.Target)+`($|[^A-Za-z0-9_])`).MatchString(req.Text) {
		return "", errors.New("whitelist target must be explicitly named in the request")
	}
	// Recheck after planning: model calls may take time or observe a revocation.
	currentUUID, err := e.Auth.Authorize(req.Player)
	if err != nil || currentUUID != uuid {
		return "", errors.New("authorization changed during planning")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	record := Record{Time: time.Now().UTC(), ID: req.ID, Player: req.Player, UUID: uuid, Action: plan.Action, Target: plan.Target, State: "started"}
	if err := e.Audit.Append(record); err != nil {
		return "", errors.New("cannot record job; operation refused")
	}
	result, opErr := e.execute(ctx, plan)
	record.Time = time.Now().UTC()
	record.State = "succeeded"
	if opErr != nil {
		record.State = "failed"
	}
	if err := e.Audit.Append(record); err != nil {
		return "", errors.New("cannot record job outcome; inspect server state before retrying")
	}
	return result, opErr
}

func ValidatePlan(plan Plan) error {
	if plan.Action != "status" && plan.Action != "whitelist_add" {
		return errors.New("operation is not allowed")
	}
	if (plan.Action == "whitelist_add" && !minecraft.ValidUsername(plan.Target)) || (plan.Action == "status" && plan.Target != "") {
		return errors.New("invalid operation arguments")
	}
	return nil
}

func (e *Engine) execute(ctx context.Context, p Plan) (string, error) {
	if p.Action == "status" {
		return e.RCON.Command(ctx, "list")
	}
	if _, err := e.RCON.Command(ctx, "whitelist add "+p.Target); err != nil {
		return "", err
	}
	list, err := e.RCON.Command(ctx, "whitelist list")
	if err != nil {
		return "", errors.New("whitelist change could not be verified")
	}
	_, names, ok := strings.Cut(list, ":")
	if ok {
		for name := range strings.SplitSeq(names, ",") {
			if strings.EqualFold(strings.TrimSpace(name), p.Target) {
				return fmt.Sprintf("Verified: %s is whitelisted.", p.Target), nil
			}
		}
	}
	return "", errors.New("whitelist change could not be verified; check server state")
}

type CommandPlanner struct{}

func (CommandPlanner) Plan(_ context.Context, text string) (Plan, error) {
	parts := strings.Fields(text)
	if len(parts) < 2 || !strings.EqualFold(parts[0], "minecraft-server-agent") {
		return Plan{}, errors.New("use: minecraft-server-agent status | minecraft-server-agent whitelist <username>")
	}
	if len(parts) == 2 && strings.EqualFold(parts[1], "status") {
		return Plan{Action: "status"}, nil
	}
	if strings.EqualFold(parts[1], "whitelist") {
		if len(parts) == 3 {
			return Plan{Action: "whitelist_add", Target: parts[2]}, nil
		}
		if len(parts) == 4 && strings.EqualFold(parts[2], "add") {
			return Plan{Action: "whitelist_add", Target: parts[3]}, nil
		}
	}
	return Plan{}, errors.New("use: minecraft-server-agent status | minecraft-server-agent whitelist <username>")
}
