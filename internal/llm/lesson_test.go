package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientGeneratesQuestionLessonWithAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request completionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !strings.Contains(request.Messages[0].Content, "lição") || !strings.Contains(request.Messages[0].Content, "história") || !strings.Contains(request.Messages[1].Content, "Não faça") || !strings.Contains(request.Messages[1].Content, "sistemas operacionais") || !strings.Contains(request.Messages[1].Content, "Paginação de memória") {
			t.Fatalf("lesson prompt = %+v", request.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{Choices: []choice{{Message: message{Content: `{"title":"Paginação de memória","topic":"sistemas operacionais","subject":"Como a memória virtual usa páginas?","kind":"question","opening":"Pense como um sistema escolhe uma página.","content":"Por que a memória virtual usa páginas?","answer":"Porque permite carregar e trocar blocos de tamanho fixo com eficiência.","takeaways":["Páginas têm tamanho fixo","A tabela mapeia endereços","O disco é mais lento que a RAM"]}`}}}})
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	lesson, err := client.GenerateLesson(context.Background(), LessonRequest{
		Topic: "sistemas operacionais", Slot: "morning",
		RecentLessons: []PriorLesson{{DateKey: "2026-08-02", Slot: "evening", Topic: "sistemas operacionais", Subject: "Paginação de memória", Kind: "text", Title: "Paginação de memória"}},
	})
	if err != nil {
		t.Fatalf("GenerateLesson() error = %v", err)
	}
	if lesson.Kind != "question" || lesson.Subject == "" || lesson.Answer == "" || len(lesson.Takeaways) != 3 {
		t.Fatalf("lesson = %+v", lesson)
	}
}

func TestClientGeneratesTextLessonWithoutAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{Choices: []choice{{Message: message{Content: `{"title":"Filas e pilhas","topic":"estruturas de dados e algoritmos","subject":"Filas e pilhas na prática","kind":"text","opening":"Duas formas de organizar trabalho.","content":"Uma fila atende primeiro quem chegou antes; uma pilha atende o último elemento inserido.","takeaways":["Fila usa FIFO","Pilha usa LIFO","A escolha muda o algoritmo"]}`}}}})
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	lesson, err := client.GenerateLesson(context.Background(), LessonRequest{Topic: "estruturas de dados e algoritmos", Slot: "evening"})
	if err != nil {
		t.Fatalf("GenerateLesson() error = %v", err)
	}
	if lesson.Kind != "text" || lesson.Subject == "" || lesson.Content == "" || lesson.Answer != "" {
		t.Fatalf("lesson = %+v", lesson)
	}
}

func TestClientRejectsIncompleteLesson(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{Choices: []choice{{Message: message{Content: `{"title":"Incompleta","topic":"matemática","subject":"Pergunta incompleta","kind":"question","opening":"Pense.","content":"Qual é a resposta?","takeaways":[]}`}}}})
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	if _, err := client.GenerateLesson(context.Background(), LessonRequest{Topic: "matemática", Slot: "morning"}); err == nil {
		t.Fatal("GenerateLesson() succeeded, want incomplete lesson rejection")
	}
}

func TestClientRejectsAdministrativeSubject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{Choices: []choice{{Message: message{Content: `{"title":"Gestão","topic":"computação","subject":"Gestão de projetos","kind":"text","opening":"Abertura","content":"Conteúdo","takeaways":["um","dois","três"]}`}}}})
	}))
	defer server.Close()

	client := NewClient(server.Client(), server.URL, "test-key", "test-model")
	if _, err := client.GenerateLesson(context.Background(), LessonRequest{Topic: "computação", Slot: "morning"}); err == nil || !strings.Contains(err.Error(), "administrative") {
		t.Fatalf("GenerateLesson() error = %v, want administrative subject rejection", err)
	}
}
