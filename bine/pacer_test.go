package bine

import (
	"context"
	"errors"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestCheckPacerUsesStartToStartInterval(t *testing.T) {
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	var waits []time.Duration
	pacer := newCheckPacer(time.Second)
	pacer.now = func() time.Time { return now }
	pacer.wait = func(_ context.Context, duration time.Duration) error {
		waits = append(waits, duration)
		now = now.Add(duration)
		return nil
	}

	assert.NilError(t, pacer.Wait(t.Context()))
	assert.Equal(t, len(waits), 0)

	// Time spent checking the first binary counts toward the interval.
	now = now.Add(250 * time.Millisecond)
	assert.NilError(t, pacer.Wait(t.Context()))
	assert.DeepEqual(t, waits, []time.Duration{750 * time.Millisecond})

	// A slow check does not add another delay once the interval has elapsed.
	now = now.Add(2 * time.Second)
	assert.NilError(t, pacer.Wait(t.Context()))
	assert.Equal(t, len(waits), 1)
}

func TestCheckPacerHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := newCheckPacer(time.Hour).Wait(ctx)
	assert.Assert(t, errors.Is(err, context.Canceled))
}

func TestCheckPacerHonorsCancellationWhileWaiting(t *testing.T) {
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	pacer := newCheckPacer(time.Hour)
	pacer.now = func() time.Time { return now }
	assert.NilError(t, pacer.Wait(t.Context()))

	ctx, cancel := context.WithCancel(t.Context())
	pacer.wait = func(ctx context.Context, duration time.Duration) error {
		cancel()
		return waitForDuration(ctx, duration)
	}

	err := pacer.Wait(ctx)
	assert.Assert(t, errors.Is(err, context.Canceled))
}

type timedLatestProvider struct {
	calls *[]time.Time
}

func (p timedLatestProvider) downloadURL(*bin) (string, error) {
	return "", nil
}

func (p timedLatestProvider) latestVersion(context.Context, *bin) (string, error) {
	*p.calls = append(*p.calls, time.Now())

	return "2.0.0", nil
}

func TestListBinsAppliesCheckInterval(t *testing.T) {
	const interval = 20 * time.Millisecond
	var calls []time.Time
	provider := timedLatestProvider{calls: &calls}
	b := &Bine{checkInterval: interval}
	bins := []*bin{
		{Name: "first", Version: "1.0.0", provider: provider},
		{Name: "second", Version: "1.0.0", provider: provider},
	}

	items, err := b.listBins(t.Context(), bins, false, true)
	assert.NilError(t, err)
	assert.Equal(t, len(items), 2)
	assert.Equal(t, len(calls), 2)
	assert.Assert(t, calls[1].Sub(calls[0]) >= interval)
}
