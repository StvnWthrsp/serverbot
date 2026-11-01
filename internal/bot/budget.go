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
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	if p.MaxCallsPerHour < 1 {
		return Plan{}, errors.New("inference limit must be positive")
	}
	j, err := OpenJournal(p.Path)
	if err != nil {
		return Plan{}, errors.New("cannot open inference ledger; check its permissions and lock")
	}
	now := time.Now().UTC()
	if len(j.calls) >= p.MaxCallsPerHour {
		j.Close()
		return Plan{}, ErrInferenceLimit
	}
	err = j.Append(Record{Time: now, ID: rand.Text(), State: "inference_started"})
	closeErr := j.Close()
	if err != nil || closeErr != nil {
		return Plan{}, errors.New("cannot reserve inference attempt; no model call made")
	}
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	return p.Planner.Plan(ctx, text)
}
