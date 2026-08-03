package news

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeProvider struct {
	key     string
	name    string
	stories []Story
	err     error
}

func (f fakeProvider) Key() string { return f.key }

func (f fakeProvider) Name() string { return f.name }

func (f fakeProvider) TopStories(context.Context, int) ([]Story, error) {
	return f.stories, f.err
}

func TestAggregatorInterleavesSourcesAndAppliesGlobalLimit(t *testing.T) {
	aggregator := NewAggregator(
		fakeProvider{key: "hacker_news", name: "Hacker News", stories: []Story{
			{ID: "hacker_news:1", Source: "hacker_news", SourceName: "Hacker News", URL: "https://news.ycombinator.com/item?id=1"},
			{ID: "hacker_news:2", Source: "hacker_news", SourceName: "Hacker News", URL: "https://news.ycombinator.com/item?id=2"},
			{ID: "hacker_news:3", Source: "hacker_news", SourceName: "Hacker News", URL: "https://news.ycombinator.com/item?id=3"},
		}},
		fakeProvider{key: "ieee_spectrum", name: "IEEE Spectrum", stories: []Story{
			{ID: "ieee_spectrum:a", Source: "ieee_spectrum", SourceName: "IEEE Spectrum", URL: "https://spectrum.ieee.org/a"},
			{ID: "ieee_spectrum:b", Source: "ieee_spectrum", SourceName: "IEEE Spectrum", URL: "https://spectrum.ieee.org/b"},
			{ID: "ieee_spectrum:c", Source: "ieee_spectrum", URL: "https://spectrum.ieee.org/c"},
		}},
	)

	stories, err := aggregator.TopStories(context.Background(), 4)
	if err != nil {
		t.Fatalf("TopStories() error = %v", err)
	}
	if got := []string{stories[0].ID, stories[1].ID, stories[2].ID, stories[3].ID}; strings.Join(got, ",") != "hacker_news:1,ieee_spectrum:a,hacker_news:2,ieee_spectrum:b" {
		t.Fatalf("story order = %v, want fair round-robin", got)
	}
}

func TestAggregatorContinuesWhenOneSourceFails(t *testing.T) {
	aggregator := NewAggregator(
		fakeProvider{key: "hacker_news", name: "Hacker News", err: errors.New("Hacker News unavailable")},
		fakeProvider{key: "ieee_spectrum", name: "IEEE Spectrum", stories: []Story{{ID: "ieee_spectrum:a", Source: "ieee_spectrum", SourceName: "IEEE Spectrum", URL: "https://spectrum.ieee.org/a"}}},
	)

	stories, err := aggregator.TopStories(context.Background(), 10)
	if err != nil {
		t.Fatalf("TopStories() error = %v, want partial success", err)
	}
	if len(stories) != 1 || stories[0].ID != "ieee_spectrum:a" {
		t.Fatalf("stories = %+v, want the healthy source item", stories)
	}
}

func TestAggregatorFailsWhenAllSourcesFail(t *testing.T) {
	aggregator := NewAggregator(
		fakeProvider{key: "hacker_news", name: "Hacker News", err: errors.New("HN unavailable")},
		fakeProvider{key: "ieee_spectrum", name: "IEEE Spectrum", err: errors.New("IEEE unavailable")},
	)

	if _, err := aggregator.TopStories(context.Background(), 10); err == nil {
		t.Fatal("TopStories() succeeded, want all-source failure")
	}
}

func TestAggregatorDeduplicatesCanonicalURLs(t *testing.T) {
	aggregator := NewAggregator(
		fakeProvider{key: "first", name: "First", stories: []Story{{ID: "first:1", URL: "https://example.test/story#section"}}},
		fakeProvider{key: "second", name: "Second", stories: []Story{{ID: "second:1", URL: "https://example.test/story"}}},
	)

	stories, err := aggregator.TopStories(context.Background(), 10)
	if err != nil {
		t.Fatalf("TopStories() error = %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("len(stories) = %d, want duplicate URL removed", len(stories))
	}
}

func TestIEEESpectrumClientParsesLatestRSSItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel>
			<item><title>Robotics update</title><link>https://spectrum.ieee.org/robotics</link><description><![CDATA[<p>Short article excerpt.</p>]]></description><guid>robotics-guid</guid></item>
			<item><title>Missing link</title><description>ignored</description></item>
			<item><title>External link</title><link>https://example.test/external</link><guid>external-guid</guid></item>
		</channel></rss>`))
	}))
	defer server.Close()

	client := NewIEEESpectrumClient(server.Client(), server.URL)
	stories, err := client.TopStories(context.Background(), 10)
	if err != nil {
		t.Fatalf("TopStories() error = %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("len(stories) = %d, want 1", len(stories))
	}
	story := stories[0]
	if story.ID != "ieee_spectrum:robotics-guid" || story.SourceName != "IEEE Spectrum" || story.Permalink != story.URL {
		t.Fatalf("story identity = %+v", story)
	}
	if story.Text != "Short article excerpt." {
		t.Fatalf("story text = %q, want stripped description", story.Text)
	}
}

func TestIEEESpectrumClientRejectsOversizedRSS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxRSSBodyBytes+1)))
	}))
	defer server.Close()

	client := NewIEEESpectrumClient(server.Client(), server.URL)
	if _, err := client.TopStories(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("TopStories() error = %v, want size-limit rejection", err)
	}
}
