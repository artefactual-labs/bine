package bine

import (
	"context"
	"time"
)

type checkPacer struct {
	interval  time.Duration
	lastStart time.Time
	now       func() time.Time
	wait      func(context.Context, time.Duration) error
}

func newCheckPacer(interval time.Duration) *checkPacer {
	return &checkPacer{
		interval: interval,
		now:      time.Now,
		wait:     waitForDuration,
	}
}

// Wait blocks until the next check may start. Time spent performing the
// previous check counts toward the interval.
func (p *checkPacer) Wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.interval <= 0 {
		return nil
	}

	now := p.now()
	if !p.lastStart.IsZero() {
		if delay := p.interval - now.Sub(p.lastStart); delay > 0 {
			if err := p.wait(ctx, delay); err != nil {
				return err
			}
			now = p.now()
		}
	}
	p.lastStart = now

	return nil
}

func waitForDuration(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
