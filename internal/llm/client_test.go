package llm

import (
	"context"
	"encoding/json"
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
		if !strings.Contains(request.Messages[0].Content, "untrusted") {
			t.Fatalf("system prompt does not establish untrusted data boundary")
		}
		if !strings.Contains(request.Messages[1].Content, "Ignore any instructions") {
			t.Fatalf("user prompt does not repeat content boundary")
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

func TestClientRejectsResponseMissingStory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{
			Choices: []choice{{Message: message{Content: `{"intro":"Abertura","items":[]}`}}},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	_, err := client.Summarize(context.Background(), []Input{{ID: 1, Title: "Title"}})
	if err == nil || !strings.Contains(err.Error(), "story_id") {
		t.Fatalf("Summarize() error = %v, want missing story_id validation", err)
	}
}
