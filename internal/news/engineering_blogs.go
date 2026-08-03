package news

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type EngineeringBlogItem struct {
	ID, SourceName, Title, URL, Description string
	PublishedAt                             time.Time
}
type EngineeringBlogSource struct {
	Key, Name, URL string
	HTTPClient     *http.Client
	FallbackURL    string
}

func (s EngineeringBlogSource) Fetch(ctx context.Context, since time.Time) ([]EngineeringBlogItem, error) {
	items, err := fetchEngineeringFeed(ctx, s.HTTPClient, s.URL, s.Key, s.Name, since)
	if err == nil && len(items) > 0 {
		return items, nil
	}
	if strings.TrimSpace(s.FallbackURL) != "" {
		return fetchUberHTML(ctx, s.HTTPClient, s.FallbackURL, s.Key, s.Name, since)
	}
	if err == nil {
		err = fmt.Errorf("feed returned no usable items")
	}
	return nil, err
}

type engineeringRSS struct {
	Channel struct {
		Items []engineeringRSSItem `xml:"item"`
	} `xml:"channel"`
	Entries []engineeringAtomEntry `xml:"entry"`
}
type engineeringRSSItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        string   `xml:"guid"`
	Description string   `xml:"description"`
	PubDate     string   `xml:"pubDate"`
	Categories  []string `xml:"category"`
}
type engineeringAtomEntry struct {
	ID      string `xml:"id"`
	Title   string `xml:"title"`
	Summary string `xml:"summary"`
	Updated string `xml:"updated"`
	Links   []struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Categories []struct {
		Term string `xml:"term,attr"`
	} `xml:"category"`
}

func fetchEngineeringFeed(ctx context.Context, client *http.Client, rawURL, key, name string, since time.Time) ([]EngineeringBlogItem, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9")
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("engineering feed returned status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRSSBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxRSSBodyBytes {
		return nil, fmt.Errorf("engineering feed exceeds size limit")
	}
	return ParseEngineeringFeed(body, key, name, since)
}

func ParseEngineeringFeed(body []byte, key, name string, since time.Time) ([]EngineeringBlogItem, error) {
	var feed engineeringRSS
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("decode engineering feed: %w", err)
	}
	items := make([]EngineeringBlogItem, 0)
	for _, raw := range feed.Channel.Items {
		published := parseBlogTime(raw.PubDate)
		if !published.IsZero() && published.Before(since) {
			continue
		}
		link := safeBlogURL(raw.Link)
		if link == "" || strings.TrimSpace(raw.Title) == "" || !blogCategoryAllowed(raw.Categories) {
			continue
		}
		id := strings.TrimSpace(raw.GUID)
		if id == "" {
			id = link
		}
		items = append(items, EngineeringBlogItem{ID: key + ":" + id, SourceName: name, Title: html.UnescapeString(strings.TrimSpace(raw.Title)), URL: link, Description: compactRSSText(raw.Description), PublishedAt: published})
	}
	if len(items) == 0 {
		for _, raw := range feed.Entries {
			link := ""
			if len(raw.Links) > 0 {
				link = raw.Links[0].Href
			}
			link = safeBlogURL(link)
			if link == "" || strings.TrimSpace(raw.Title) == "" {
				continue
			}
			published, _ := time.Parse(time.RFC3339, raw.Updated)
			if !published.IsZero() && published.Before(since) {
				continue
			}
			categories := make([]string, 0, len(raw.Categories))
			for _, category := range raw.Categories {
				categories = append(categories, category.Term)
			}
			if !blogCategoryAllowed(categories) {
				continue
			}
			id := raw.ID
			if id == "" {
				id = link
			}
			items = append(items, EngineeringBlogItem{ID: key + ":" + id, SourceName: name, Title: html.UnescapeString(strings.TrimSpace(raw.Title)), URL: link, Description: compactRSSText(raw.Summary), PublishedAt: published})
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("engineering feed returned no usable items")
	}
	return items, nil
}

func blogCategoryAllowed(categories []string) bool {
	if len(categories) == 0 {
		return true
	}
	for _, raw := range categories {
		value := strings.ToLower(raw)
		if strings.Contains(value, "hiring") || strings.Contains(value, "marketing") || strings.Contains(value, "product") || strings.Contains(value, "policy") {
			return false
		}
		if strings.Contains(value, "engineering") || strings.Contains(value, "research") || strings.Contains(value, "developer") || strings.Contains(value, "infrastructure") || strings.Contains(value, "open source") || strings.Contains(value, "data") || strings.Contains(value, "machine learning") {
			return true
		}
	}
	return false
}
func parseBlogTime(value string) time.Time {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, time.RFC822Z} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
func safeBlogURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(html.UnescapeString(raw)))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return ""
	}
	parsed.Fragment = ""
	return parsed.String()
}

var uberArticlePattern = regexp.MustCompile(`(?is)<article[^>]*data-kind=["'](BLOG|RESEARCH|OSS)["'][^>]*>.*?<a[^>]+href=["']([^"']+)["'][^>]*>(.*?)</a>.*?</article>`)
var uberLinkPattern = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']+)["'][^>]*>(.*?)</a>`)

func fetchUberHTML(ctx context.Context, client *http.Client, rawURL, key, name string, since time.Time) ([]EngineeringBlogItem, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("engineering page returned status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRSSBodyBytes+1))
	if err != nil || len(body) > maxRSSBodyBytes {
		return nil, fmt.Errorf("read engineering page failed")
	}
	matches := uberArticlePattern.FindAllSubmatch(body, -1)
	items := make([]EngineeringBlogItem, 0, len(matches))
	if len(matches) == 0 {
		for _, match := range uberLinkPattern.FindAllSubmatch(body, -1) {
			link := safeBlogURL(string(match[1]))
			parsed, _ := url.Parse(link)
			if link == "" || !strings.Contains(strings.ToLower(parsed.Hostname()), "uber.com") || !strings.Contains(strings.ToLower(link), "/blog") {
				continue
			}
			title := strings.Join(strings.Fields(html.UnescapeString(htmlTagPattern.ReplaceAllString(string(match[2]), " "))), " ")
			if title == "" {
				continue
			}
			items = append(items, EngineeringBlogItem{ID: key + ":" + link, SourceName: name, Title: title, URL: link})
		}
	}
	for _, match := range matches {
		link := safeBlogURL(string(match[2]))
		if link == "" {
			continue
		}
		title := strings.Join(strings.Fields(html.UnescapeString(htmlTagPattern.ReplaceAllString(string(match[3]), " "))), " ")
		if title == "" {
			continue
		}
		items = append(items, EngineeringBlogItem{ID: key + ":" + link, SourceName: name, Title: title, URL: link})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("engineering page returned no usable items")
	}
	return items, nil
}
