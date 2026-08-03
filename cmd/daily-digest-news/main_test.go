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

type learningStartupRunnerStub struct {
	calls   int
	called  chan struct{}
	release chan struct{}
}

func (s *learningStartupRunnerStub) RunStartup(context.Context) error {
	s.calls++
	close(s.called)
	<-s.release
	return nil
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

func TestStartStartupLearningRunsImmediatelyAndOnce(t *testing.T) {
	runner := &learningStartupRunnerStub{called: make(chan struct{}), release: make(chan struct{})}
	done := startStartupLearning(context.Background(), runner)
	select {
	case <-runner.called:
	case <-time.After(2 * time.Second):
		t.Fatal("startup learning did not begin immediately")
	}
	select {
	case <-done:
		t.Fatal("startup learning completed before runner was released")
	default:
	}
	close(runner.release)
	<-done
	if runner.calls != 1 {
		t.Fatalf("RunStartup calls = %d, want 1", runner.calls)
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

func TestHTTPClientsUseDedicatedTimeouts(t *testing.T) {
	generalClient, aiClient := newHTTPClients()
	if generalClient == aiClient {
		t.Fatal("general and AI clients share the same instance")
	}
	if generalClient.Timeout != 20*time.Second {
		t.Fatalf("general HTTP timeout = %s, want 20s", generalClient.Timeout)
	}
	if aiClient.Timeout != 5*time.Minute {
		t.Fatalf("AI HTTP timeout = %s, want 5m", aiClient.Timeout)
	}
}
