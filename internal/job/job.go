package job

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/email"
	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	"github.com/ramonpaolo/daily-digest-news/internal/news"
)

type Source interface {
	TopStories(context.Context, int) ([]news.Story, error)
}

type Extractor interface {
	Extract(context.Context, string) (string, error)
}

type Summarizer interface {
	Summarize(context.Context, []llm.Input) (llm.Digest, error)
}

type Mailer interface {
	Send(context.Context, email.Message) error
}

type Runner struct {
	source     Source
	extractor  Extractor
	summarizer Summarizer
	mailer     Mailer
	limit      int
	location   *time.Location
	now        func() time.Time
	sleep      func(context.Context, time.Duration) error

	mu          sync.Mutex
	running     bool
	lastSuccess string
}

func NewRunner(source Source, extractor Extractor, summarizer Summarizer, mailer Mailer) *Runner {
	return &Runner{
		source:     source,
		extractor:  extractor,
		summarizer: summarizer,
		mailer:     mailer,
		limit:      10,
		location:   time.UTC,
		now:        time.Now,
		sleep:      sleepContext,
	}
}

func (r *Runner) SetLimit(limit int) {
	if limit > 0 {
		r.limit = limit
	}
}

func (r *Runner) SetLocation(location *time.Location) {
	if location != nil {
		r.location = location
	}
}

func (r *Runner) Run(ctx context.Context) error {
	return r.run(ctx, false)
}

func (r *Runner) RunOnce(ctx context.Context) error {
	return r.run(ctx, true)
}

func (r *Runner) run(ctx context.Context, force bool) error {
	now := r.now().In(r.location)
	dateKey := now.Format("2006-01-02")
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("digest job is already running")
	}
	if !force && r.lastSuccess == dateKey {
		r.mu.Unlock()
		return fmt.Errorf("digest already sent for %s", dateKey)
	}
	r.running = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	stories, err := retry(r, ctx, func(ctx context.Context) ([]news.Story, error) {
		return r.source.TopStories(ctx, r.limit)
	})
	if err != nil {
		return fmt.Errorf("collect stories: %w", err)
	}
	inputs := make([]llm.Input, 0, len(stories))
	for _, story := range stories {
		articleText := story.Text
		if story.URL != "" {
			if extracted, extractErr := retry(r, ctx, func(ctx context.Context) (string, error) {
				return r.extractor.Extract(ctx, story.URL)
			}); extractErr == nil {
				articleText = extracted
			}
		}
		inputs = append(inputs, llm.Input{
			ID:          story.ID,
			Title:       story.Title,
			URL:         story.URL,
			HNText:      story.Text,
			ArticleText: articleText,
			Score:       story.Score,
			Comments:    story.Descendants,
		})
	}
	digest, err := retry(r, ctx, func(ctx context.Context) (llm.Digest, error) {
		return r.summarizer.Summarize(ctx, inputs)
	})
	if err != nil {
		return fmt.Errorf("summarize stories: %w", err)
	}
	message, err := email.Render(stories, digest, now)
	if err != nil {
		return fmt.Errorf("render email: %w", err)
	}
	if err := retryVoid(r, ctx, func(ctx context.Context) error { return r.mailer.Send(ctx, message) }); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	r.mu.Lock()
	r.lastSuccess = dateKey
	r.mu.Unlock()
	return nil
}

func retry[T any](r *Runner, ctx context.Context, operation func(context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		value, err := operation(ctx)
		if err == nil {
			return value, nil
		}
		lastErr = err
		if attempt < 2 {
			if err := r.sleep(ctx, time.Duration(1<<attempt)*time.Second); err != nil {
				return zero, err
			}
		}
	}
	return zero, lastErr
}

func retryVoid(r *Runner, ctx context.Context, operation func(context.Context) error) error {
	_, err := retry(r, ctx, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, operation(ctx)
	})
	return err
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
