package fetch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	readability "github.com/go-shiori/go-readability"
)

const (
	maxBodyBytes = 2 * 1024 * 1024
	maxTextChars = 12_000
)

type HostValidator func(context.Context, string) error

type Fetcher struct {
	httpClient *http.Client
	validate   HostValidator
}

func NewFetcher(httpClient *http.Client, validator HostValidator) *Fetcher {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	enforcePrivate := validator == nil
	if validator == nil {
		validator = validatePublicHost
	}
	client := *httpClient
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if configured, ok := httpClient.Transport.(*http.Transport); ok {
		transport = configured.Clone()
	}
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return safeDialContext(ctx, network, address, validator, enforcePrivate)
	}
	client.Transport = transport
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return validator(req.Context(), req.URL.Hostname())
	}
	return &Fetcher{httpClient: &client, validate: validator}
}

func safeDialContext(ctx context.Context, network, address string, validator HostValidator, enforcePrivate bool) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid article destination")
	}
	if err := validator(ctx, host); err != nil {
		return nil, err
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("article host cannot be resolved")
	}
	for _, address := range addresses {
		if enforcePrivate && blockedIP(address.IP) {
			continue
		}
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
	}
	return nil, fmt.Errorf("article host is private or local")
}

func (f *Fetcher) Extract(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid article URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("article URL scheme must be http or https")
	}
	if parsed.Hostname() == "" || parsed.User != nil {
		return "", fmt.Errorf("article URL host is invalid")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := f.validate(ctx, parsed.Hostname()); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create article request: %w", err)
	}
	req.Header.Set("User-Agent", "daily-digest-news/1.0 (+https://github.com/ramonpaolo/daily-digest-news)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,text/plain;q=0.8")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch article: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("article returned status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return "", fmt.Errorf("read article: %w", err)
	}
	if len(body) > maxBodyBytes {
		return "", fmt.Errorf("article exceeds %d-byte limit", maxBodyBytes)
	}
	if !utf8.Valid(body) {
		return "", fmt.Errorf("article is not valid UTF-8")
	}
	article, err := readability.FromReader(strings.NewReader(string(body)), parsed)
	if err != nil {
		return "", fmt.Errorf("extract readable content: %w", err)
	}
	text := strings.Join(strings.Fields(article.TextContent), " ")
	if text == "" {
		return "", fmt.Errorf("article has no readable text")
	}
	return truncateRunes(text, maxTextChars), nil
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

func validatePublicHost(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if blockedIP(ip) {
			return fmt.Errorf("article host is private or local")
		}
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("article host cannot be resolved")
	}
	for _, address := range addresses {
		if blockedIP(address.IP) {
			return fmt.Errorf("article host is private or local")
		}
	}
	return nil
}

func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}
