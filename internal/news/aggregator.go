package news

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Provider interface {
	Key() string
	Name() string
	TopStories(context.Context, int) ([]Story, error)
}

type Aggregator struct {
	providers []Provider
	logf      func(string, ...any)
}

func NewAggregator(providers ...Provider) *Aggregator {
	return &Aggregator{providers: append([]Provider(nil), providers...), logf: log.Printf}
}

func (a *Aggregator) SetLogger(logf func(string, ...any)) {
	if logf != nil {
		a.logf = logf
	}
}

func (a *Aggregator) TopStories(ctx context.Context, limit int) ([]Story, error) {
	if limit < 1 {
		return nil, fmt.Errorf("story limit must be positive")
	}
	if len(a.providers) == 0 {
		return nil, fmt.Errorf("no news providers configured")
	}

	type result struct {
		index   int
		stories []Story
		err     error
	}
	results := make(chan result, len(a.providers))
	var waitGroup sync.WaitGroup
	for index, provider := range a.providers {
		waitGroup.Add(1)
		go func(index int, provider Provider) {
			defer waitGroup.Done()
			started := time.Now()
			a.logf("component=news event=provider_start provider=%s limit=%d", provider.Key(), limit)
			stories, err := provider.TopStories(ctx, limit)
			if err != nil {
				a.logf("component=news event=provider_failed provider=%s duration_ms=%d error=%q", provider.Key(), time.Since(started).Milliseconds(), safeError(err))
			} else {
				a.logf("component=news event=provider_success provider=%s stories=%d duration_ms=%d", provider.Key(), len(stories), time.Since(started).Milliseconds())
			}
			results <- result{index: index, stories: stories, err: err}
		}(index, provider)
	}
	waitGroup.Wait()
	close(results)

	byProvider := make([][]Story, len(a.providers))
	failed := 0
	for item := range results {
		if item.err != nil || len(item.stories) == 0 {
			failed++
			continue
		}
		for storyIndex := range item.stories {
			story := &item.stories[storyIndex]
			if story.Source == "" {
				story.Source = a.providers[item.index].Key()
			}
			if story.SourceName == "" {
				story.SourceName = a.providers[item.index].Name()
			}
			if story.ID == "" {
				story.ID = story.Source + ":" + canonicalStoryURL(story.URL)
			}
		}
		byProvider[item.index] = item.stories
	}
	if failed == len(a.providers) {
		return nil, fmt.Errorf("all news providers failed")
	}
	if failed > 0 {
		a.logf("component=news event=aggregate_partial failed_providers=%d healthy_providers=%d", failed, len(a.providers)-failed)
	}

	selected := make([]Story, 0, limit)
	seen := make(map[string]struct{}, limit)
	for round := 0; len(selected) < limit; round++ {
		added := false
		for providerIndex := range byProvider {
			if round >= len(byProvider[providerIndex]) {
				continue
			}
			story := byProvider[providerIndex][round]
			key := canonicalStoryURL(story.URL)
			if key == "" {
				key = story.ID
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			selected = append(selected, story)
			added = true
			if len(selected) == limit {
				break
			}
		}
		if !added {
			break
		}
	}
	a.logf("component=news event=aggregate_success providers=%d stories=%d limit=%d", len(a.providers), len(selected), limit)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no usable stories returned")
	}
	return selected, nil
}

func canonicalStoryURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return ""
	}
	parsed.Fragment = ""
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String()
}
