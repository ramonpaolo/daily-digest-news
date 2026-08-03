package news

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
}

func NewClient(httpClient *http.Client, baseURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{httpClient: httpClient, baseURL: strings.TrimRight(baseURL, "/")}
}

func (c *Client) TopStories(ctx context.Context, limit int) ([]Story, error) {
	if limit < 1 {
		return nil, fmt.Errorf("story limit must be positive")
	}

	var ids []int
	if err := c.getJSON(ctx, "/v0/topstories.json", &ids); err != nil {
		return nil, fmt.Errorf("fetch top stories: %w", err)
	}

	stories := make([]Story, 0, limit)
	for _, id := range ids {
		if len(stories) == limit {
			break
		}
		var item Item
		if err := c.getJSON(ctx, fmt.Sprintf("/v0/item/%d.json", id), &item); err != nil {
			continue
		}
		if item.ID == 0 || item.Type != "story" || item.Dead || item.Deleted || strings.TrimSpace(item.Title) == "" {
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
		return nil, fmt.Errorf("no usable stories returned")
	}
	return stories, nil
}

func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	return nil
}
