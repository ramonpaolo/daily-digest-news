package main

import (
	"context"
	"testing"
	"time"
)

type startupRunnerStub struct {
	calls   int
	called  chan struct{}
	release chan struct{}
}

func (s *startupRunnerStub) RunOnce(context.Context) error {
	s.calls++
	close(s.called)
	<-s.release
	return nil
}

func TestStartStartupDigestRunsImmediatelyAndOnce(t *testing.T) {
	runner := &startupRunnerStub{
		called:  make(chan struct{}),
		release: make(chan struct{}),
	}

	done := startStartupDigest(context.Background(), runner)
	select {
	case <-runner.called:
	case <-time.After(2 * time.Second):
		t.Fatal("startup digest did not begin immediately")
	}

	select {
	case <-done:
		t.Fatal("startup digest completed before the runner was released")
	default:
	}

	close(runner.release)
	<-done
	if runner.calls != 1 {
		t.Fatalf("RunOnce calls = %d, want 1", runner.calls)
	}
}

func TestParseSchedule(t *testing.T) {
	hour, minute, err := parseSchedule("08:05")
	if err != nil || hour != 8 || minute != 5 {
		t.Fatalf("parseSchedule() = %d:%d, %v; want 8:5", hour, minute, err)
	}
	if _, _, err := parseSchedule("invalid"); err == nil {
		t.Fatal("parseSchedule(invalid) succeeded, want error")
	}
}