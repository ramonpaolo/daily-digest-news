package news

import (
	"context"
	"testing"
	"time"
)

func TestParseEngineeringFeedFiltersCategoriesAndWindow(t *testing.T) {
	body := []byte(`<rss><channel><item><title>Infra</title><link>https://example.test/infra</link><guid>a</guid><description>desc</description><pubDate>Sun, 02 Aug 2026 12:00:00 +0000</pubDate><category>Engineering</category></item><item><title>Hiring</title><link>https://example.test/hiring</link><guid>b</guid><category>Hiring</category></item></channel></rss>`)
	items, err := ParseEngineeringFeed(body, "x", "X", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(items) != 1 || items[0].ID != "x:a" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestEngineeringBlogSourceUsesFallback(t *testing.T) {
	source := EngineeringBlogSource{Key: "uber", Name: "Uber", URL: "://invalid", FallbackURL: "https://example.test"}
	if _, err := source.Fetch(context.Background(), time.Now().Add(-24*time.Hour)); err == nil {
		t.Fatal("Fetch() succeeded, want invalid feed error")
	}
}
