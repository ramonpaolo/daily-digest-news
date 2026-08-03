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
		ID: 1, Title: "<script>alert(1)</script>", URL: "javascript:alert(1)", Score: 10,
	}}, llm.Digest{
		Intro: "Intro",
		Items: []llm.Item{{StoryID: 1, Summary: "<b>summary</b>", WhyItMatters: "why"}},
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
	_, err := Render([]news.Story{{ID: 1, Title: "Title"}}, llm.Digest{Intro: "Intro"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "story_id") {
		t.Fatalf("Render() error = %v, want missing story_id", err)
	}
}
