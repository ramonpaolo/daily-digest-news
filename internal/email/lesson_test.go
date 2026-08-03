package email

import (
	"strings"
	"testing"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/llm"
)

func TestRenderLessonBuildsSeparateReadableEmail(t *testing.T) {
	message, err := RenderLesson(llm.Lesson{
		Title:     "<Pilhas e filas>",
		Topic:     "estruturas de dados e algoritmos",
		Subject:   "Como pilhas e filas organizam dados",
		Kind:      "question",
		Opening:   "Uma pergunta para pensar.",
		Content:   "Por que uma pilha funciona como LIFO?",
		Answer:    "Porque o último item inserido é o primeiro removido.",
		Takeaways: []string{"LIFO", "FIFO", "Escolha depende do problema"},
	}, time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC), "morning")
	if err != nil {
		t.Fatalf("RenderLesson() error = %v", err)
	}
	if !strings.Contains(message.Subject, "Como pilhas e filas") || !strings.Contains(message.Subject, "estruturas de dados") {
		t.Fatalf("Subject = %q", message.Subject)
	}
	if !strings.Contains(message.TextBody, "Resposta comentada") || !strings.Contains(message.TextBody, "LIFO") {
		t.Fatalf("TextBody = %q", message.TextBody)
	}
	if strings.Contains(message.HTMLBody, "<Pilhas e filas>") {
		t.Fatalf("HTMLBody contains unescaped title: %q", message.HTMLBody)
	}
}

func TestRenderLessonOmitsAnswerForText(t *testing.T) {
	message, err := RenderLesson(llm.Lesson{
		Title: "Memória virtual", Topic: "sistemas operacionais", Subject: "Como a memória virtual funciona", Kind: "text",
		Opening: "Uma visão geral.", Content: "A memória virtual cria uma camada de endereçamento.",
		Takeaways: []string{"endereços", "páginas", "tradução"},
	}, time.Now(), "evening")
	if err != nil {
		t.Fatalf("RenderLesson() error = %v", err)
	}
	if strings.Contains(message.TextBody, "Resposta comentada") {
		t.Fatalf("TextBody = %q, want no answer section", message.TextBody)
	}
}
