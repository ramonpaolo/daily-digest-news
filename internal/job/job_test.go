package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/email"
	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	"github.com/ramonpaolo/daily-digest-news/internal/news"
)

type fakeSource struct {
	stories []news.Story
}

func (f fakeSource) TopStories(context.Context, int) ([]news.Story, error) { return f.stories, nil }

type fakeExtractor struct{}

func (fakeExtractor) Extract(context.Context, string) (string, error) { return "article text", nil }

type fakeSummarizer struct {
	inputs []llm.Input
}

func (f *fakeSummarizer) Summarize(_ context.Context, inputs []llm.Input) (llm.Digest, error) {
	f.inputs = inputs
	return llm.Digest{Intro: "Intro", Items: []llm.Item{{StoryID: 1, Summary: "Summary", WhyItMatters: "Why"}}}, nil
}

type fakeMailer struct {
	sent   []email.Message
	failed int
}

func (f *fakeMailer) Send(_ context.Context, message email.Message) error {
	if f.failed > 0 {
		f.failed--
		return errors.New("temporary SMTP error")
	}
	f.sent = append(f.sent, message)
	return nil
}

func TestRunnerBuildsAndSendsOneDigestPerDay(t *testing.T) {
	summarizer := &fakeSummarizer{}
	mailer := &fakeMailer{}
	runner := NewRunner(fakeSource{stories: []news.Story{{ID: 1, Title: "Title", URL: "https://example.test", Score: 5}}}, fakeExtractor{}, summarizer, mailer)
	runner.now = func() time.Time { return time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC) }
	runner.sleep = func(context.Context, time.Duration) error { return nil }

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(summarizer.inputs) != 1 || summarizer.inputs[0].ArticleText != "article text" {
		t.Fatalf("summarizer inputs = %+v, want extracted article", summarizer.inputs)
	}
	if len(mailer.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(mailer.sent))
	}
	if err := runner.Run(context.Background()); err == nil {
		t.Fatal("second Run() succeeded, want same-day guard")
	}
	if len(mailer.sent) != 1 {
		t.Fatalf("sent messages after duplicate = %d, want 1", len(mailer.sent))
	}
}

func TestRunnerRetriesTransientMailFailureThreeTimes(t *testing.T) {
	mailer := &fakeMailer{failed: 2}
	runner := NewRunner(fakeSource{stories: []news.Story{{ID: 1, Title: "Title", URL: "https://example.test"}}}, fakeExtractor{}, &fakeSummarizer{}, mailer)
	runner.now = func() time.Time { return time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC) }
	runner.sleep = func(context.Context, time.Duration) error { return nil }

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v, want retry recovery", err)
	}
	if len(mailer.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(mailer.sent))
	}
}
