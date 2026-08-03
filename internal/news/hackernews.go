package news

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

const defaultUserAgent = "daily-digest-news/1.0 (+https://github.com/ramonpaolo/daily-digest-news)"

type Item struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Text        string `json:"text"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
	Dead        bool   `json:"dead"`
	Deleted     bool   `json:"deleted"`
}

type Story struct {
	ID          int
	Title       string
	URL         string
	Text        string
	Score       int
	Descendants int
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	logf       func(string, ...any)
}

func NewClient(httpClient *http.Client, baseURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{httpClient: httpClient, baseURL: strings.TrimRight(baseURL, "/"), logf: log.Printf}
}

func (c *Client) SetLogger(logf func(string, ...any)) {
	if logf != nil {
		c.logf = logf
	}
}

func (c *Client) TopStories(ctx context.Context, limit int) ([]Story, error) {
	if limit < 1 {
		c.logf("component=news event=top_stories_rejected reason=invalid_limit limit=%d", limit)
		return nil, fmt.Errorf("story limit must be positive")
	}

	started := time.Now()
	c.logf("component=news event=top_stories_start limit=%d", limit)
	var ids []int
	if err := c.getJSON(ctx, "/v0/topstories.json", &ids); err != nil {
		c.logf("component=news event=top_stories_failed duration_ms=%d error=%q", time.Since(started).Milliseconds(), safeError(err))
		return nil, fmt.Errorf("fetch top stories: %w", err)
	}
	c.logf("component=news event=top_story_ids_success ids=%d duration_ms=%d", len(ids), time.Since(started).Milliseconds())

	stories := make([]Story, 0, limit)
	for _, id := range ids {
		if len(stories) == limit {
			break
		}
		var item Item
		if err := c.getJSON(ctx, fmt.Sprintf("/v0/item/%d.json", id), &item); err != nil {
			c.logf("component=news event=story_item_failed story_id=%d error=%q", id, safeError(err))
			continue
		}
		if item.ID == 0 || item.Type != "story" || item.Dead || item.Deleted || strings.TrimSpace(item.Title) == "" {
			c.logf("component=news event=story_item_skipped story_id=%d reason=not_usable", id)
			continue
		}
		stories = append(stories, Story{
			ID:          item.ID,
			Title:       item.Title,
			URL:         item.URL,
			Text:        item.Text,
			Score:       item.Score,
			Descendants: item.Descendants,
		})
	}
	if len(stories) == 0 {
		c.logf("component=news event=top_stories_failed duration_ms=%d error=%q", time.Since(started).Milliseconds(), "no usable stories returned")
		return nil, fmt.Errorf("no usable stories returned")
	}
	c.logf("component=news event=top_stories_success stories=%d duration_ms=%d", len(stories), time.Since(started).Milliseconds())
	return stories, nil
}

func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	started := time.Now()
	c.logf("component=news event=request_start path=%s", path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		c.logf("component=news event=request_create_failed path=%s duration_ms=%d error=%q", path, time.Since(started).Milliseconds(), safeError(err))
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logf("component=news event=request_failed path=%s duration_ms=%d error=%q", path, time.Since(started).Milliseconds(), safeError(err))
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logf("component=news event=response_failed path=%s status=%d duration_ms=%d", path, resp.StatusCode, time.Since(started).Milliseconds())
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		c.logf("component=news event=response_decode_failed path=%s status=%d duration_ms=%d error=%q", path, resp.StatusCode, time.Since(started).Milliseconds(), safeError(err))
		return fmt.Errorf("decode JSON: %w", err)
	}
	c.logf("component=news event=response_success path=%s status=%d duration_ms=%d", path, resp.StatusCode, time.Since(started).Milliseconds())
	return nil
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.Join(strings.Fields(err.Error()), " ")
	if len(value) > 240 {
		return value[:240] + "…"
	}
	return value
}
