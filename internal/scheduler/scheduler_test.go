package scheduler

import (
	"context"
	"testing"
	"time"
)

type fakeRunner struct{ calls int }

func (f *fakeRunner) Run(context.Context) error {
	f.calls++
	return nil
}

func TestRunStopsBeforeWaitingWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &fakeRunner{}
	scheduler := New(runner, time.UTC, 8, 0)
	scheduler.now = func() time.Time { return time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC) }

	if err := scheduler.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v, want nil on cancellation", err)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}
