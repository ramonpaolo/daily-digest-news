package job

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ramonpaolo/daily-digest-news/internal/email"
	"github.com/ramonpaolo/daily-digest-news/internal/fetch"
	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	"github.com/ramonpaolo/daily-digest-news/internal/news"
	"github.com/ramonpaolo/daily-digest-news/internal/storage"
	"log"
	"sync"
	"time"
)

type EngineeringBlogGenerator interface {
	SelectEngineeringBlogs(context.Context, []llm.BlogCandidate) ([]string, error)
	SummarizeEngineeringBlogs(context.Context, []llm.BlogInput) (llm.BlogDigest, error)
}
type EngineeringBlogsRunner struct {
	generator EngineeringBlogGenerator
	mailer    Mailer
	store     *storage.Store
	fetcher   *fetch.Fetcher
	sources   []news.EngineeringBlogSource
	location  *time.Location
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
	logf      Logf
	mu        sync.Mutex
	running   bool
}

func NewEngineeringBlogsRunner(g EngineeringBlogGenerator, m Mailer, s *storage.Store, f *fetch.Fetcher, sources []news.EngineeringBlogSource) *EngineeringBlogsRunner {
	return &EngineeringBlogsRunner{generator: g, mailer: m, store: s, fetcher: f, sources: sources, location: time.UTC, now: time.Now, sleep: sleepContext, logf: log.Printf}
}
func (r *EngineeringBlogsRunner) SetLocation(v *time.Location) {
	if v != nil {
		r.location = v
	}
}
func (r *EngineeringBlogsRunner) SetLogger(v Logf) {
	if v != nil {
		r.logf = v
	}
}
func (r *EngineeringBlogsRunner) Run(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("engineering blogs are already running")
	}
	r.running = true
	r.mu.Unlock()
	defer func() { r.mu.Lock(); r.running = false; r.mu.Unlock() }()
	now := r.now().In(r.location)
	if now.Weekday() != time.Sunday {
		return nil
	}
	week := now.Format("2006-01-02")
	claim, err := r.store.BeginEngineeringBlogRun(ctx, week, now)
	if err != nil || claim.AlreadySent {
		return err
	}
	seen, err := r.store.SentEngineeringBlogItems(ctx)
	if err != nil {
		return err
	}
	since := now.Add(-7 * 24 * time.Hour)
	var candidates []news.EngineeringBlogItem
	for _, source := range r.sources {
		items, sourceErr := source.Fetch(ctx, since)
		if sourceErr != nil {
			r.logf("component=engineering_blogs event=source_failed source=%s", source.Name)
			continue
		}
		for _, item := range items {
			if _, ok := seen[item.ID]; !ok {
				candidates = append(candidates, item)
			}
		}
	}
	if len(candidates) == 0 {
		return r.store.MarkEngineeringBlogFailed(ctx, claim.ID, "no_new_articles", fmt.Errorf("no new articles"), now)
	}
	meta := make([]llm.BlogCandidate, 0, len(candidates))
	for _, item := range candidates {
		meta = append(meta, llm.BlogCandidate{ID: item.ID, SourceName: item.SourceName, Title: item.Title, URL: item.URL, Description: item.Description})
	}
	selected, err := retry(r, ctx, "engineering_blog_selection", func(ctx context.Context) ([]string, error) { return r.generator.SelectEngineeringBlogs(ctx, meta) })
	if err != nil {
		return err
	}
	byID := map[string]news.EngineeringBlogItem{}
	for _, item := range candidates {
		byID[item.ID] = item
	}
	inputs := make([]llm.BlogInput, 0, len(selected))
	stored := make([]struct{ Key, Source, Title, URL, Published string }, 0, len(selected))
	for _, id := range selected {
		item, ok := byID[id]
		if !ok {
			continue
		}
		text := item.Description
		if r.fetcher != nil {
			if extracted, extractErr := r.fetcher.Extract(ctx, item.URL); extractErr == nil {
				text = extracted
			}
		}
		inputs = append(inputs, llm.BlogInput{ID: item.ID, SourceName: item.SourceName, Title: item.Title, URL: item.URL, ArticleText: text})
		stored = append(stored, struct{ Key, Source, Title, URL, Published string }{item.ID, item.SourceName, item.Title, item.URL, item.PublishedAt.Format(time.RFC3339)})
	}
	digest, err := retry(r, ctx, "engineering_blog_summary", func(ctx context.Context) (llm.BlogDigest, error) {
		return r.generator.SummarizeEngineeringBlogs(ctx, inputs)
	})
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(digest)
	if err := r.store.SaveEngineeringBlogRun(ctx, claim.ID, string(raw), stored, now); err != nil {
		return err
	}
	message, err := email.RenderEngineeringBlogs(digest, now)
	if err != nil {
		return err
	}
	if err := retryVoid(r, ctx, "engineering_blog_email", func(ctx context.Context) error { return r.mailer.Send(ctx, message) }); err != nil {
		return err
	}
	return r.store.MarkEngineeringBlogSent(ctx, claim.ID, r.now().In(r.location))
}
func (r *EngineeringBlogsRunner) retryLogf(f string, a ...any) { r.logf(f, a...) }
func (r *EngineeringBlogsRunner) retrySleep(c context.Context, d time.Duration) error {
	return r.sleep(c, d)
}
