package scheduler

import (
	"context"
	"log"
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
			return nil
		default:
		}
		next := schedule.Next(s.now().In(s.location), s.hour, s.minute)
		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}

		if err := s.runner.Run(ctx); err != nil {
			s.logf("digest execution failed: %v", err)
		} else {
			s.logf("digest execution completed")
		}
	}
}
