package job

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/email"
	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	"github.com/ramonpaolo/daily-digest-news/internal/storage"
)

type LessonGenerator interface {
	GenerateLesson(context.Context, llm.LessonRequest) (llm.Lesson, error)
}

type LearningRunner struct {
	generator LessonGenerator
	mailer    Mailer
	store     *storage.Store
	topics    []string
	location  *time.Location
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
	logf      Logf

	mu      sync.Mutex
	running bool
}

func NewLearningRunner(generator LessonGenerator, mailer Mailer, store *storage.Store, topics []string) *LearningRunner {
	cleanTopics := make([]string, 0, len(topics))
	for _, topic := range topics {
		topic = strings.Join(strings.Fields(topic), " ")
		if topic != "" {
			cleanTopics = append(cleanTopics, topic)
		}
	}
	return &LearningRunner{
		generator: generator,
		mailer:    mailer,
		store:     store,
		topics:    cleanTopics,
		location:  time.UTC,
		now:       time.Now,
		sleep:     sleepContext,
		logf:      log.Printf,
	}
}

func (r *LearningRunner) SetLogger(logf Logf) {
	if logf != nil {
		r.logf = logf
	}
}

func (r *LearningRunner) SetLocation(location *time.Location) {
	if location != nil {
		r.location = location
	}
}

func (r *LearningRunner) retryLogf(format string, args ...any) { r.logf(format, args...) }

func (r *LearningRunner) retrySleep(ctx context.Context, duration time.Duration) error {
	return r.sleep(ctx, duration)
}

func (r *LearningRunner) RunStartup(ctx context.Context) error {
	return r.run(ctx, "startup", true)
}

func (r *LearningRunner) RunSlot(ctx context.Context, slot string) error {
	slot = strings.TrimSpace(slot)
	if slot == "" {
		return fmt.Errorf("learning slot is required")
	}
	return r.run(ctx, slot, false)
}

type lessonRequestSnapshot struct {
	Topic         string            `json:"topic"`
	Slot          string            `json:"slot"`
	PromptVersion string            `json:"prompt_version"`
	Prompt        string            `json:"prompt"`
	RecentLessons []llm.PriorLesson `json:"recent_lessons"`
}

func (r *LearningRunner) run(ctx context.Context, slot string, force bool) (runErr error) {
	started := time.Now()
	now := r.now().In(r.location)
	dateKey := now.Format("2006-01-02")
	r.logf("component=learning event=lesson_start force=%t date=%s slot=%s", force, dateKey, slot)
	if r.store == nil {
		return fmt.Errorf("learning storage is not configured")
	}
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		r.logf("component=learning event=lesson_skip reason=already_running slot=%s", slot)
		return fmt.Errorf("learning lesson is already running")
	}
	r.running = true
	r.mu.Unlock()
	skipped := false
	var lessonID int64
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
		if skipped {
			r.logf("component=learning event=lesson_skip reason=already_sent date=%s slot=%s", dateKey, slot)
			return
		}
		if runErr != nil {
			r.logf("component=learning event=lesson_failed date=%s slot=%s duration_ms=%d error=%q", dateKey, slot, time.Since(started).Milliseconds(), safeError(runErr))
		} else {
			r.logf("component=learning event=lesson_success date=%s slot=%s duration_ms=%d", dateKey, slot, time.Since(started).Milliseconds())
		}
	}()

	if err := r.store.Cleanup(ctx, now); err != nil {
		return fmt.Errorf("cleanup learning history: %w", err)
	}
	if err := r.store.SyncInterests(ctx, r.topics, now); err != nil {
		return fmt.Errorf("sync learning interests: %w", err)
	}
	history, err := r.store.RecentLessons(ctx, 8)
	if err != nil {
		return fmt.Errorf("load learning history: %w", err)
	}
	recent := make([]llm.PriorLesson, 0, len(history))
	for _, prior := range history {
		recent = append(recent, llm.PriorLesson{DateKey: prior.DateKey, Slot: prior.Slot, Topic: prior.Topic, Kind: prior.Kind, Title: prior.Title})
	}
	claim, err := r.store.BeginLesson(ctx, dateKey, slot, `{}`, now)
	if err != nil {
		return fmt.Errorf("begin learning lesson: %w", err)
	}
	if claim.AlreadySent {
		skipped = true
		return nil
	}
	lessonID = claim.ID
	request := llm.LessonRequest{Topic: claim.Topic, Slot: slot, RecentLessons: recent}
	prompt := llm.BuildLessonPrompt(request)
	snapshot, err := json.Marshal(lessonRequestSnapshot{Topic: request.Topic, Slot: request.Slot, PromptVersion: "lesson-v2", Prompt: prompt, RecentLessons: recent})
	if err != nil {
		return r.failLesson(ctx, lessonID, "request", fmt.Errorf("encode lesson request snapshot: %w", err), now)
	}
	if err := r.store.UpdateRequest(ctx, lessonID, string(snapshot), now); err != nil {
		return r.failLesson(ctx, lessonID, "storage", err, now)
	}
	r.logf("component=learning event=generate_start topic=%s slot=%s history=%d", claim.Topic, slot, len(recent))
	lesson, err := retry(r, ctx, "learning_generation", func(ctx context.Context) (llm.Lesson, error) {
		return r.generator.GenerateLesson(ctx, request)
	})
	if err != nil {
		return r.failLesson(ctx, lessonID, "llm", fmt.Errorf("generate learning lesson: %w", err), now)
	}
	r.logf("component=learning event=generate_success topic=%s kind=%s takeaways=%d", claim.Topic, lesson.Kind, len(lesson.Takeaways))
	if err := r.store.SaveGenerated(ctx, lessonID, lesson, now); err != nil {
		return r.failLesson(ctx, lessonID, "storage", err, now)
	}
	message, err := email.RenderLesson(lesson, now, slot)
	if err != nil {
		return r.failLesson(ctx, lessonID, "render", fmt.Errorf("render learning email: %w", err), now)
	}
	r.logf("component=learning event=email_start topic=%s text_bytes=%d html_bytes=%d", claim.Topic, len(message.TextBody), len(message.HTMLBody))
	if err := retryVoid(r, ctx, "learning_email", func(ctx context.Context) error { return r.mailer.Send(ctx, message) }); err != nil {
		return r.failLesson(ctx, lessonID, "smtp", fmt.Errorf("send learning email: %w", err), now)
	}
	if err := r.store.MarkSent(ctx, lessonID, r.now().In(r.location)); err != nil {
		return r.failLesson(ctx, lessonID, "storage", err, now)
	}
	r.logf("component=learning event=email_success topic=%s", claim.Topic)
	return nil
}

func (r *LearningRunner) failLesson(ctx context.Context, id int64, phase string, failure error, now time.Time) error {
	if err := r.store.MarkFailed(ctx, id, phase, failure, now); err != nil {
		r.logf("component=learning event=storage_failure phase=%s error=%q", phase, safeError(err))
	}
	return failure
}
