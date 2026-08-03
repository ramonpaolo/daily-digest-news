package news

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const defaultIEEESpectrumFeed = "https://spectrum.ieee.org/feeds/feed.rss"

const maxRSSBodyBytes = 2 << 20

type IEEESpectrumClient struct {
	httpClient *http.Client
	feedURL    string
	logf       func(string, ...any)
}

func NewIEEESpectrumClient(httpClient *http.Client, feedURL string) *IEEESpectrumClient {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	if strings.TrimSpace(feedURL) == "" {
		feedURL = defaultIEEESpectrumFeed
	}
	return &IEEESpectrumClient{httpClient: httpClient, feedURL: feedURL, logf: log.Printf}
}

func (c *IEEESpectrumClient) Key() string { return "ieee_spectrum" }

func (c *IEEESpectrumClient) Name() string { return "IEEE Spectrum" }

func (c *IEEESpectrumClient) SetLogger(logf func(string, ...any)) {
	if logf != nil {
		c.logf = logf
	}
}

func (c *IEEESpectrumClient) TopStories(ctx context.Context, limit int) ([]Story, error) {
	if limit < 1 {
		return nil, fmt.Errorf("story limit must be positive")
	}
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create IEEE Spectrum request: %w", err)
	}
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9")
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch IEEE Spectrum feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("IEEE Spectrum feed returned status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRSSBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read IEEE Spectrum feed: %w", err)
	}
	if len(body) > maxRSSBodyBytes {
		return nil, fmt.Errorf("IEEE Spectrum feed exceeds size limit")
	}
	var feed rssDocument
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("decode IEEE Spectrum feed: %w", err)
	}
	stories := make([]Story, 0, limit)
	for _, item := range feed.Channel.Items {
		if len(stories) == limit {
			break
		}
		story, ok := c.storyFromRSS(item)
		if !ok {
			continue
		}
		stories = append(stories, story)
	}
	if len(stories) == 0 {
		return nil, fmt.Errorf("IEEE Spectrum feed returned no usable stories")
	}
	c.logf("component=news event=ieee_feed_success stories=%d duration_ms=%d", len(stories), time.Since(started).Milliseconds())
	return stories, nil
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	GUID        string `xml:"guid"`
}

func (c *IEEESpectrumClient) storyFromRSS(item rssItem) (Story, bool) {
	title := strings.TrimSpace(html.UnescapeString(item.Title))
	link := strings.TrimSpace(html.UnescapeString(item.Link))
	parsed, err := url.Parse(link)
	if title == "" || err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || !strings.EqualFold(parsed.Hostname(), "spectrum.ieee.org") {
		return Story{}, false
	}
	guid := strings.TrimSpace(item.GUID)
	if guid == "" {
		guid = link
	}
	return Story{
		ID:         c.Key() + ":" + guid,
		Source:     c.Key(),
		SourceName: c.Name(),
		Title:      title,
		URL:        link,
		Permalink:  link,
		Text:       compactRSSText(item.Description),
	}, true
}

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

func compactRSSText(value string) string {
	value = html.UnescapeString(value)
	value = htmlTagPattern.ReplaceAllString(value, " ")
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) > 12000 {
		return string([]rune(value)[:12000])
	}
	return value
}
