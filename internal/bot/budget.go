package bot

import (
	"context"
	"crypto/rand"
	"errors"
	"time"
)

var ErrCooldown = errors.New("please wait before sending another request")
var ErrInferenceLimit = errors.New("hourly inference limit reached; try again later")

// BudgetPlanner reserves each attempt durably before contacting a model, including previews and failures.
type BudgetPlanner struct {
	Planner         Planner
	Path            string
	MaxCallsPerHour int
}

func (p BudgetPlanner) Plan(ctx context.Context, text string) (Plan, error) {
	if err := ReserveAttempt(ctx, p.Path, p.MaxCallsPerHour); err != nil {
		return Plan{}, err
	}
	return p.Planner.Plan(ctx, text)
}

func ReserveAttempt(ctx context.Context, path string, maxPerHour int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if maxPerHour < 1 {
		return errors.New("request limit must be positive")
	}
	j, err := OpenJournal(path)
	if err != nil {
		return errors.New("cannot open request ledger; check its permissions and lock")
	}
	now := time.Now().UTC()
	if len(j.calls) >= maxPerHour {
		j.Close()
		return ErrInferenceLimit
	}
	err = j.Append(Record{Time: now, ID: rand.Text(), State: "inference_started"})
	closeErr := j.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot reserve request; no agent/model call made")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
