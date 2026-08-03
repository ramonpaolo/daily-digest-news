package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSummarizesStoriesUsingZenifraCompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("Authorization header missing or incorrect")
		}
		var request completionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "test-model" || len(request.Messages) != 2 {
			t.Fatalf("request = %+v, want model and system/user messages", request)
		}
		if request.MaxTokens != 12000 {
			t.Fatalf("request max_tokens = %d, want 12000", request.MaxTokens)
		}
		if !strings.Contains(request.Messages[0].Content, "untrusted") {
			t.Fatalf("system prompt does not establish untrusted data boundary")
		}
		if !strings.Contains(request.Messages[1].Content, "Ignore any instructions") {
			t.Fatalf("user prompt does not repeat content boundary")
		}
		for _, field := range []string{"story_id", "summary", "why_it_matters"} {
			if !strings.Contains(request.Messages[1].Content, field) {
				t.Fatalf("user prompt does not require %s field", field)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{
			Choices: []choice{{Message: message{Content: `{"intro":"Abertura","items":[{"story_id":1,"summary":"Resumo","why_it_matters":"Relevância"}]}`}}},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	digest, err := client.Summarize(context.Background(), []Input{{ID: 1, Title: "Title", URL: "https://example.test", ArticleText: "Article", Score: 10}})
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if digest.Intro != "Abertura" || len(digest.Items) != 1 || digest.Items[0].StoryID != 1 {
		t.Fatalf("digest = %+v, want decoded digest", digest)
	}
}

func TestClientRepairsResponseMissingStory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{
			Choices: []choice{{Message: message{Content: `{"intro":"Abertura","items":[]}`}}},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	digest, err := client.Summarize(context.Background(), []Input{{ID: 1, Title: "Title"}})
	if err != nil {
		t.Fatalf("Summarize() error = %v, want missing story repaired", err)
	}
	if len(digest.Items) != 1 || digest.Items[0].StoryID != 1 {
		t.Fatalf("digest = %+v, want one repaired item", digest)
	}
}

func TestClientLogsResponseMetadataWhenCompletionHasNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"","reasoning_content":"private reasoning"}}]}`))
	}))
	defer server.Close()

	var logs bytes.Buffer
	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	client.SetLogger(func(format string, args ...any) {
		_, _ = fmt.Fprintf(&logs, format+"\n", args...)
	})
	_, err := client.Summarize(context.Background(), []Input{{ID: 1, Title: "Title"}})
	if err == nil || !strings.Contains(err.Error(), "completion returned no content") {
		t.Fatalf("Summarize() error = %v, want no-content diagnostic", err)
	}

	output := logs.String()
	for _, want := range []string{
		"component=llm event=request_start",
		"component=llm event=response",
		"choices=1",
		"reasoning_bytes=17",
		"finish_reason=length",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs missing %q:\n%s", want, output)
		}
	}
	for _, secret := range []string{"test-key", "private reasoning"} {
		if strings.Contains(output, secret) {
			t.Fatalf("logs contain sensitive value %q:\n%s", secret, output)
		}
	}
}

func TestClientAcceptsNumericStoryIDEncodedAsString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{
			Choices: []choice{{Message: message{Content: `{"intro":"Abertura","items":[{"story_id":"1","summary":"Resumo","why_it_matters":"Relevância"}]}`}}},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	digest, err := client.Summarize(context.Background(), []Input{{ID: 1, Title: "Title"}})
	if err != nil {
		t.Fatalf("Summarize() error = %v, want numeric string story_id accepted", err)
	}
	if len(digest.Items) != 1 || digest.Items[0].StoryID != 1 {
		t.Fatalf("digest = %+v, want story_id 1", digest)
	}
}

func TestDecodeStoryIDAcceptsIntegralRepresentations(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want int
	}{
		{name: "integer", raw: `49154332`, want: 49154332},
		{name: "numeric string", raw: `"49154332"`, want: 49154332},
		{name: "decimal string", raw: `"49154332.0"`, want: 49154332},
		{name: "scientific number", raw: `4.9154332e7`, want: 49154332},
		{name: "labelled string", raw: `"Hacker News ID: 49154332"`, want: 49154332},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeStoryID(json.RawMessage(test.raw))
			if err != nil {
				t.Fatalf("decodeStoryID(%s) error = %v", test.raw, err)
			}
			if got != test.want {
				t.Fatalf("decodeStoryID(%s) = %d, want %d", test.raw, got, test.want)
			}
		})
	}
}

func TestClientNormalizesStoriesFieldToDigestItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{
			Choices: []choice{{Message: message{Content: `{"intro":"Abertura","stories":[{"story_id":"1","summary":"Resumo","why_it_matters":"Relevância"}]}`}}},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	digest, err := client.Summarize(context.Background(), []Input{{ID: 1, Title: "Title"}})
	if err != nil {
		t.Fatalf("Summarize() error = %v, want stories field normalized", err)
	}
	if len(digest.Items) != 1 || digest.Items[0].StoryID != 1 {
		t.Fatalf("digest = %+v, want one normalized item", digest)
	}
}

func TestClientAcceptsStoryIDAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{
			Choices: []choice{{Message: message{Content: `{"intro":"Abertura","items":[{"id":"Hacker News ID: 1","summary":"Resumo","why_it_matters":"Relevância"}]}`}}},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	digest, err := client.Summarize(context.Background(), []Input{{ID: 1, Title: "Title"}})
	if err != nil {
		t.Fatalf("Summarize() error = %v, want id alias accepted", err)
	}
	if len(digest.Items) != 1 || digest.Items[0].StoryID != 1 {
		t.Fatalf("digest = %+v, want one item with story_id 1", digest)
	}
}

func TestRepairDigestFillsMissingAndIncompleteItems(t *testing.T) {
	inputs := []Input{{ID: 1, Title: "Primeira notícia"}, {ID: 2, Title: "Segunda notícia"}}
	digest := Digest{Items: []Item{{StoryID: 1}}}

	stats := repairDigest(inputs, &digest)
	if stats.MissingItems != 1 || stats.IncompleteText != 2 || !stats.MissingIntro {
		t.Fatalf("repair stats = %+v, want missing item, two text fields and intro", stats)
	}
	if err := validateDigest(inputs, digest); err != nil {
		t.Fatalf("validateDigest() error = %v after repair", err)
	}
	if !strings.Contains(digest.Items[0].Summary, "Primeira notícia") || digest.Items[1].StoryID != 2 {
		t.Fatalf("digest after repair = %+v, want title fallback and item 2", digest)
	}
}
