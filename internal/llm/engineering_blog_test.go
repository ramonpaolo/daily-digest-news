package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientSelectsAndSummarizesEngineeringBlogs(t *testing.T) {
	responses := []string{
		`{"selected_ids":["uber:a","netflix:b"]}`,
		`{"intro":"Dois artigos técnicos.","items":[{"story_id":"uber:a","summary":"Resumo técnico","why_it_matters":"Explica uma decisão","key_ideas":["particionamento"],"tradeoffs":"complexidade"},{"story_id":"netflix:b","summary":"Outro resumo","why_it_matters":"Mostra escala","key_ideas":["streaming"],"tradeoffs":"custo"}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response string
		if len(responses) == 0 {
			t.Fatal("unexpected request")
		}
		response, responses = responses[0], responses[1:]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{Choices: []choice{{Message: message{Content: response}}}})
	}))
	defer server.Close()
	client := NewClient(server.Client(), server.URL, "key", "model")
	candidates := []BlogCandidate{{ID: "uber:a", SourceName: "Uber", Title: "A", URL: "https://example.test/a"}, {ID: "netflix:b", SourceName: "Netflix", Title: "B", URL: "https://example.test/b"}}
	selected, err := client.SelectEngineeringBlogs(context.Background(), candidates)
	if err != nil || len(selected) != 2 {
		t.Fatalf("selected=%+v err=%v", selected, err)
	}
	digest, err := client.SummarizeEngineeringBlogs(context.Background(), []BlogInput{{ID: "uber:a", SourceName: "Uber", Title: "A", URL: candidates[0].URL, ArticleText: "texto"}, {ID: "netflix:b", SourceName: "Netflix", Title: "B", URL: candidates[1].URL, ArticleText: "texto"}})
	if err != nil || len(digest.Items) != 2 || digest.Items[0].StoryID != "uber:a" {
		t.Fatalf("digest=%+v err=%v", digest, err)
	}
}
