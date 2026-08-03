package scheduler

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/schedule"
)

type Runner interface {
	Run(context.Context) error
}

type Scheduler struct {
	runner   Runner
	location *time.Location
	hour     int
	minute   int
	now      func() time.Time
	logf     func(string, ...any)
}

func New(runner Runner, location *time.Location, hour, minute int) *Scheduler {
	if location == nil {
		location = time.UTC
	}
	return &Scheduler{
		runner:   runner,
		location: location,
		hour:     hour,
		minute:   minute,
		now:      time.Now,
		logf:     log.Printf,
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			s.logf("component=scheduler event=stop reason=context_done")
			return nil
		default:
		}
		next := schedule.Next(s.now().In(s.location), s.hour, s.minute)
		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}
		s.logf("component=scheduler event=next_run scheduled_at=%s wait_ms=%d", next.Format(time.RFC3339), wait.Milliseconds())
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.logf("component=scheduler event=wait_cancelled reason=context_done")
			return nil
		case <-timer.C:
		}

		runStarted := time.Now()
		s.logf("component=scheduler event=run_start scheduled_at=%s", next.Format(time.RFC3339))
		if err := s.runner.Run(ctx); err != nil {
			s.logf("component=scheduler event=run_failed duration_ms=%d error=%q", time.Since(runStarted).Milliseconds(), safeError(err))
		} else {
			s.logf("component=scheduler event=run_success duration_ms=%d", time.Since(runStarted).Milliseconds())
		}
	}
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.Join(strings.Fields(err.Error()), " ")
	value = redactURLTokens(value)
	if len(value) > 240 {
		return value[:240] + "…"
	}
	return value
}

func redactURLTokens(value string) string {
	for _, scheme := range []string{"http://", "https://"} {
		for {
			start := strings.Index(value, scheme)
			if start < 0 {
				break
			}
			end := start
			for end < len(value) && !strings.ContainsRune(" \t\r\n\"'<>[]()", rune(value[end])) {
				end++
			}
			value = value[:start] + "[URL_REDACTED]" + value[end:]
		}
	}
	return value
}
