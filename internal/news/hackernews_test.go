package news

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientReturnsRankedStoriesAndSkipsDeadItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v0/topstories.json":
			_ = json.NewEncoder(w).Encode([]int{1, 2, 3, 4})
		case "/v0/item/1.json":
			_ = json.NewEncoder(w).Encode(Item{ID: 1, Type: "story", Title: "First", URL: "https://example.test/first", Score: 101, Descendants: 9})
		case "/v0/item/2.json":
			_ = json.NewEncoder(w).Encode(Item{ID: 2, Type: "story", Title: "Dead", Dead: true})
		case "/v0/item/3.json":
			_ = json.NewEncoder(w).Encode(Item{ID: 3, Type: "story", Title: "Ask HN", Text: "A question", Score: 80})
		case "/v0/item/4.json":
			_ = json.NewEncoder(w).Encode(Item{ID: 4, Type: "comment", Title: "Not a story"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL)
	stories, err := client.TopStories(context.Background(), 2)
	if err != nil {
		t.Fatalf("TopStories() error = %v", err)
	}
	if len(stories) != 2 {
		t.Fatalf("len(stories) = %d, want 2", len(stories))
	}
	if stories[0].ID != "hacker_news:1" || stories[1].ID != "hacker_news:3" {
		t.Fatalf("story IDs = %v, want [hacker_news:1 hacker_news:3]", []string{stories[0].ID, stories[1].ID})
	}
	if stories[0].Score == nil || *stories[0].Score != 101 || stories[0].Comments == nil || *stories[0].Comments != 9 {
		t.Fatalf("metadata not preserved: %+v", stories[0])
	}
	if stories[1].Text != "A question" {
		t.Fatalf("text = %q, want Ask HN text", stories[1].Text)
	}
	if stories[0].Permalink != "https://news.ycombinator.com/item?id=1" {
		t.Fatalf("permalink = %q", stories[0].Permalink)
	}
}
