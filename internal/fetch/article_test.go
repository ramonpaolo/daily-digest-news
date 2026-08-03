package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetcherExtractsReadableTextAndCapsOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Noise</title><script>alert('x')</script></head><body><nav>Menu</nav><article><h1>Article</h1><p>Useful content.</p></article></body></html>`))
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), func(context.Context, string) error { return nil })
	text, err := fetcher.Extract(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if !strings.Contains(text, "Useful content.") {
		t.Fatalf("Extract() = %q, want article content", text)
	}
	if strings.Contains(text, "alert('x')") {
		t.Fatalf("Extract() retained script content: %q", text)
	}
}

func TestFetcherRejectsLoopbackByDefault(t *testing.T) {
	fetcher := NewFetcher(nil, nil)
	_, err := fetcher.Extract(context.Background(), "http://127.0.0.1:8080/internal")
	if err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("Extract() error = %v, want private/local URL rejection", err)
	}
}

func TestFetcherRejectsNonHTTPURL(t *testing.T) {
	fetcher := NewFetcher(nil, nil)
	_, err := fetcher.Extract(context.Background(), "file:///etc/passwd")
	if err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("Extract() error = %v, want scheme rejection", err)
	}
}

func TestFetcherRevalidatesRedirectTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"?next=1", http.StatusFound)
	}))
	defer server.Close()

	calls := 0
	fetcher := NewFetcher(server.Client(), func(context.Context, string) error {
		calls++
		if calls > 2 {
			return errors.New("private or local")
		}
		return nil
	})
	_, err := fetcher.Extract(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("Extract() error = %v, want redirect validation error", err)
	}
}
