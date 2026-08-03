package job

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ramonpaolo/daily-digest-news/internal/email"
	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	"github.com/ramonpaolo/daily-digest-news/internal/storage"
	"log"
	"sync"
	"time"
)

type SystemDesignGenerator interface {
	GenerateSystemDesign(context.Context, llm.SystemDesignRequest) (llm.SystemDesignLesson, error)
}
type SystemDesignRunner struct {
	generator SystemDesignGenerator
	mailer    Mailer
	store     *storage.Store
	location  *time.Location
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
	logf      Logf
	mu        sync.Mutex
	running   bool
}

func NewSystemDesignRunner(g SystemDesignGenerator, m Mailer, s *storage.Store) *SystemDesignRunner {
	return &SystemDesignRunner{generator: g, mailer: m, store: s, location: time.UTC, now: time.Now, sleep: sleepContext, logf: log.Printf}
}
func (r *SystemDesignRunner) SetLocation(v *time.Location) {
	if v != nil {
		r.location = v
	}
}
func (r *SystemDesignRunner) SetLogger(v Logf) {
	if v != nil {
		r.logf = v
	}
}
func (r *SystemDesignRunner) Run(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("system design is already running")
	}
	r.running = true
	r.mu.Unlock()
	defer func() { r.mu.Lock(); r.running = false; r.mu.Unlock() }()
	now := r.now().In(r.location)
	claim, err := r.store.BeginSystemDesign(ctx, now.Format("2006-01-02"), now)
	if err != nil {
		return err
	}
	if claim.AlreadySent {
		return nil
	}
	subjects, err := r.store.RecentSystemDesignSubjects(ctx, 8)
	if err != nil {
		return err
	}
	lesson, err := retry(r, ctx, "system_design_generation", func(ctx context.Context) (llm.SystemDesignLesson, error) {
		return r.generator.GenerateSystemDesign(ctx, llm.SystemDesignRequest{RecentSubjects: subjects})
	})
	if err != nil {
		r.store.MarkSystemDesignFailed(ctx, claim.ID, "llm", err, now)
		return err
	}
	raw, _ := json.Marshal(lesson)
	if err := r.store.SaveSystemDesign(ctx, claim.ID, lesson.Subject, string(raw), now); err != nil {
		return err
	}
	message, err := email.RenderSystemDesign(lesson, now)
	if err != nil {
		return err
	}
	if err := retryVoid(r, ctx, "system_design_email", func(ctx context.Context) error { return r.mailer.Send(ctx, message) }); err != nil {
		r.store.MarkSystemDesignFailed(ctx, claim.ID, "smtp", err, now)
		return err
	}
	return r.store.MarkSystemDesignSent(ctx, claim.ID, r.now().In(r.location))
}
func (r *SystemDesignRunner) retryLogf(f string, a ...any) { r.logf(f, a...) }
func (r *SystemDesignRunner) retrySleep(c context.Context, d time.Duration) error {
	return r.sleep(c, d)
}
