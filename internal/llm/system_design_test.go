package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientGeneratesSystemDesignLesson(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request completionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(request.Messages[0].Content, "System Design") || !strings.Contains(request.Messages[1].Content, "replicação") {
			t.Fatalf("prompt = %+v", request.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{Choices: []choice{{Message: message{Content: `{"subject":"Replicação síncrona e assíncrona","opening":"Comece pela diferença de confirmação.","content":"A replicação mantém cópias coordenadas.","mechanics":"O líder envia eventos às réplicas.","tradeoffs":["latência","durabilidade"],"failure_modes":["réplica atrasada","partição"],"example":"Uma fila de eventos distribui mudanças.","takeaways":["consistência custa latência","atraso precisa ser medido","falhas exigem recuperação"]}`}}}})
	}))
	defer server.Close()
	lesson, err := NewClient(server.Client(), server.URL, "key", "model").GenerateSystemDesign(context.Background(), SystemDesignRequest{RecentSubjects: []string{"Cache write-through"}})
	if err != nil {
		t.Fatal(err)
	}
	if lesson.Subject == "" || lesson.Mechanics == "" || len(lesson.Tradeoffs) != 2 || len(lesson.Takeaways) != 3 {
		t.Fatalf("lesson = %+v", lesson)
	}
}
