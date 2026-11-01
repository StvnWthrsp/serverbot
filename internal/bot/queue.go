package bot

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/minecraft"
)

var ErrQueueFull = errors.New("request queue is full")
var ErrPlayerBusy = errors.New("player already has a pending request")

type queuedRequest struct {
	request  minecraft.Request
	uuid     string
	deadline time.Time
}

type Queue struct {
	pending chan queuedRequest
	auth    Authorizer
	timeout time.Duration
	handle  func(context.Context, minecraft.Request)
	mu      sync.Mutex
	players map[string]bool
}

func NewQueue(capacity int, timeout time.Duration, auth Authorizer, handle func(context.Context, minecraft.Request)) *Queue {
	return &Queue{pending: make(chan queuedRequest, capacity), auth: auth, timeout: timeout, handle: handle, players: make(map[string]bool)}
}

// Submit never waits for model/RCON work. Only authorized players can occupy queue slots.
func (q *Queue) Submit(ctx context.Context, req minecraft.Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if req.ID == "" || len(req.ID) > 128 || !minecraft.ValidUsername(req.Player) || len(req.Text) > 512 {
		return errors.New("invalid request")
	}
	uuid, err := q.auth.Authorize(req.Player)
	if err != nil {
		return err
	}
	uuid = strings.ToLower(uuid)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.players[uuid] {
		return ErrPlayerBusy
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.pending <- queuedRequest{request: req, uuid: uuid, deadline: time.Now().Add(q.timeout)}:
		q.players[uuid] = true
		return nil
	default:
		return ErrQueueFull
	}
}

func (q *Queue) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case job := <-q.pending:
			jobCtx, cancel := context.WithDeadline(ctx, job.deadline)
			if jobCtx.Err() == nil {
				q.handle(jobCtx, job.request)
			}
			cancel()
			q.mu.Lock()
			delete(q.players, job.uuid)
			q.mu.Unlock()
		}
	}
}
