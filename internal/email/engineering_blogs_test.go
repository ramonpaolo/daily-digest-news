package email

import (
	"strings"
	"testing"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/llm"
)

func TestRenderEngineeringBlogs(t *testing.T) {
	message, err := RenderEngineeringBlogs(llm.BlogDigest{Intro: "Intro", Items: []llm.BlogDigestItem{{StoryID: "uber:a", SourceName: "Uber", Title: "Sistema", URL: "https://example.test/a", Summary: "Resumo", WhyItMatters: "Importa", KeyIdeas: []string{"ideia"}, Tradeoffs: "custo"}}}, time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.Subject, "Engineering Blogs") || !strings.Contains(message.TextBody, "Uber") || !strings.Contains(message.HTMLBody, "https://example.test/a") {
		t.Fatalf("message=%+v", message)
	}
}
