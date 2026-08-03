package job

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
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

type Logf func(string, ...any)

type Runner struct {
	source     Source
	extractor  Extractor
	summarizer Summarizer
	mailer     Mailer
	limit      int
	location   *time.Location
	now        func() time.Time
	sleep      func(context.Context, time.Duration) error
	logf       Logf

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
		logf:       log.Printf,
	}
}

func (r *Runner) SetLogger(logf Logf) {
	if logf != nil {
		r.logf = logf
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

func (r *Runner) run(ctx context.Context, force bool) (runErr error) {
	started := time.Now()
	now := r.now().In(r.location)
	dateKey := now.Format("2006-01-02")
	r.logf("component=job event=digest_start force=%t date=%s limit=%d", force, dateKey, r.limit)
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		r.logf("component=job event=digest_skip reason=already_running date=%s", dateKey)
		return fmt.Errorf("digest job is already running")
	}
	if !force && r.lastSuccess == dateKey {
		r.mu.Unlock()
		r.logf("component=job event=digest_skip reason=already_sent date=%s", dateKey)
		return fmt.Errorf("digest already sent for %s", dateKey)
	}
	r.running = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
		if runErr != nil {
			r.logf("component=job event=digest_failed date=%s duration_ms=%d error=%q", dateKey, time.Since(started).Milliseconds(), safeError(runErr))
			return
		}
		r.logf("component=job event=digest_success date=%s duration_ms=%d", dateKey, time.Since(started).Milliseconds())
	}()

	sourceStarted := time.Now()
	r.logf("component=job event=source_start limit=%d", r.limit)
	stories, err := retry(r, ctx, "source", func(ctx context.Context) ([]news.Story, error) {
		return r.source.TopStories(ctx, r.limit)
	})
	if err != nil {
		r.logf("component=job event=source_failed duration_ms=%d error=%q", time.Since(sourceStarted).Milliseconds(), safeError(err))
		return fmt.Errorf("collect stories: %w", err)
	}
	r.logf("component=job event=source_success stories=%d duration_ms=%d", len(stories), time.Since(sourceStarted).Milliseconds())

	inputs := make([]llm.Input, 0, len(stories))
	inputBytes := 0
	for _, story := range stories {
		articleText := story.Text
		if story.URL != "" {
			extractStarted := time.Now()
			r.logf("component=job event=article_extract_start story_id=%d host=%s", story.ID, storyHost(story.URL))
			if extracted, extractErr := retry(r, ctx, "article_extract", func(ctx context.Context) (string, error) {
				return r.extractor.Extract(ctx, story.URL)
			}); extractErr == nil {
				articleText = extracted
				r.logf("component=job event=article_extract_success story_id=%d chars=%d duration_ms=%d", story.ID, len([]rune(extracted)), time.Since(extractStarted).Milliseconds())
			} else {
				r.logf("component=job event=article_extract_fallback story_id=%d chars=%d duration_ms=%d error=%q", story.ID, len([]rune(story.Text)), time.Since(extractStarted).Milliseconds(), safeError(extractErr))
			}
		} else {
			r.logf("component=job event=article_extract_skipped story_id=%d reason=no_url chars=%d", story.ID, len([]rune(story.Text)))
		}
		inputBytes += len(story.Title) + len(story.URL) + len(story.Text) + len(articleText)
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

	summaryStarted := time.Now()
	r.logf("component=job event=summary_start stories=%d input_bytes=%d", len(inputs), inputBytes)
	digest, err := retry(r, ctx, "summary", func(ctx context.Context) (llm.Digest, error) {
		return r.summarizer.Summarize(ctx, inputs)
	})
	if err != nil {
		r.logf("component=job event=summary_failed duration_ms=%d error=%q", time.Since(summaryStarted).Milliseconds(), safeError(err))
		return fmt.Errorf("summarize stories: %w", err)
	}
	r.logf("component=job event=summary_success items=%d duration_ms=%d", len(digest.Items), time.Since(summaryStarted).Milliseconds())

	renderStarted := time.Now()
	message, err := email.Render(stories, digest, now)
	if err != nil {
		r.logf("component=job event=email_render_failed duration_ms=%d error=%q", time.Since(renderStarted).Milliseconds(), safeError(err))
		return fmt.Errorf("render email: %w", err)
	}
	r.logf("component=job event=email_render_success text_bytes=%d html_bytes=%d duration_ms=%d", len(message.TextBody), len(message.HTMLBody), time.Since(renderStarted).Milliseconds())

	sendStarted := time.Now()
	r.logf("component=job event=email_start text_bytes=%d html_bytes=%d", len(message.TextBody), len(message.HTMLBody))
	if err := retryVoid(r, ctx, "email", func(ctx context.Context) error { return r.mailer.Send(ctx, message) }); err != nil {
		r.logf("component=job event=email_failed duration_ms=%d error=%q", time.Since(sendStarted).Milliseconds(), safeError(err))
		return fmt.Errorf("send email: %w", err)
	}
	r.logf("component=job event=email_success duration_ms=%d", time.Since(sendStarted).Milliseconds())
	r.mu.Lock()
	r.lastSuccess = dateKey
	r.mu.Unlock()
	return nil
}

func retry[T any](r *Runner, ctx context.Context, phase string, operation func(context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		attemptStarted := time.Now()
		r.logf("component=job event=retry_attempt_start phase=%s attempt=%d", phase, attempt+1)
		value, err := operation(ctx)
		if err == nil {
			r.logf("component=job event=retry_attempt_success phase=%s attempt=%d duration_ms=%d", phase, attempt+1, time.Since(attemptStarted).Milliseconds())
			return value, nil
		}
		lastErr = err
		r.logf("component=job event=retry_attempt_failed phase=%s attempt=%d duration_ms=%d error=%q", phase, attempt+1, time.Since(attemptStarted).Milliseconds(), safeError(err))
		if attempt < 2 {
			backoff := time.Duration(1<<attempt) * time.Second
			r.logf("component=job event=retry_backoff phase=%s attempt=%d backoff_ms=%d", phase, attempt+1, backoff.Milliseconds())
			if err := r.sleep(ctx, backoff); err != nil {
				r.logf("component=job event=retry_backoff_failed phase=%s attempt=%d error=%q", phase, attempt+1, safeError(err))
				return zero, err
			}
		}
	}
	return zero, lastErr
}

func retryVoid(r *Runner, ctx context.Context, phase string, operation func(context.Context) error) error {
	_, err := retry(r, ctx, phase, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, operation(ctx)
	})
	return err
}

func storyHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return "invalid"
	}
	return parsed.Hostname()
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
