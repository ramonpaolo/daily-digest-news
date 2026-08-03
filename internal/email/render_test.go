package email

import (
	"strings"
	"testing"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	"github.com/ramonpaolo/daily-digest-news/internal/news"
)

func TestRenderEscapesUntrustedContentAndBuildsBothBodies(t *testing.T) {
	message, err := Render([]news.Story{{
		ID: "hacker_news:1", Source: "hacker_news", SourceName: "Hacker News", Title: "<script>alert(1)</script>", URL: "javascript:alert(1)", Permalink: "https://news.ycombinator.com/item?id=1", Score: intPointer(10),
	}}, llm.Digest{
		Intro: "Intro",
		Items: []llm.Item{{StoryID: "hacker_news:1", Summary: "<b>summary</b>", WhyItMatters: "why"}},
	}, time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(message.Subject, "03/08/2026") {
		t.Fatalf("Subject = %q, want formatted date", message.Subject)
	}
	if !strings.Contains(message.TextBody, "<script>alert(1)</script>") {
		t.Fatalf("TextBody should preserve readable text, got %q", message.TextBody)
	}
	if strings.Contains(message.HTMLBody, "<script>alert(1)</script>") || strings.Contains(message.HTMLBody, "<b>summary</b>") {
		t.Fatalf("HTMLBody contains unescaped content: %q", message.HTMLBody)
	}
	if !strings.Contains(message.HTMLBody, "https://news.ycombinator.com/item?id=1") {
		t.Fatalf("HTMLBody lacks safe Hacker News fallback link: %q", message.HTMLBody)
	}
}

func TestRenderRejectsMissingDigestItem(t *testing.T) {
	_, err := Render([]news.Story{{ID: "hacker_news:1", Title: "Title"}}, llm.Digest{Intro: "Intro"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "story_id") {
		t.Fatalf("Render() error = %v, want missing story_id", err)
	}
}

func TestRenderLabelsSourceAndOmitsUnavailableMetrics(t *testing.T) {
	message, err := Render([]news.Story{{
		ID: "ieee_spectrum:abc", SourceName: "IEEE Spectrum", Title: "Robotics", URL: "https://spectrum.ieee.org/robotics", Permalink: "https://spectrum.ieee.org/robotics",
	}}, llm.Digest{
		Intro: "Intro",
		Items: []llm.Item{{StoryID: "ieee_spectrum:abc", Summary: "Resumo", WhyItMatters: "Relevância"}},
	}, time.Now())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(message.TextBody, "Fonte: IEEE Spectrum") {
		t.Fatalf("TextBody = %q, want source label", message.TextBody)
	}
	if strings.Contains(message.TextBody, "Score:") || strings.Contains(message.TextBody, "Comentários:") {
		t.Fatalf("TextBody = %q, want unavailable metrics omitted", message.TextBody)
	}
}

func intPointer(value int) *int { return &value }
