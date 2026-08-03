package email

import (
	"strings"
	"testing"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/llm"
)

func TestRenderSystemDesign(t *testing.T) {
	message, err := RenderSystemDesign(llm.SystemDesignLesson{Subject: "Replicação", Opening: "Abertura", Content: "Explicação", Mechanics: "Mecânica", Tradeoffs: []string{"latência"}, FailureModes: []string{"atraso"}, Example: "Exemplo", Takeaways: []string{"um", "dois", "três"}}, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.Subject, "Replicação") || !strings.Contains(message.TextBody, "Trade-offs") || !strings.Contains(message.HTMLBody, "Mecânica") {
		t.Fatalf("message=%+v", message)
	}
}
