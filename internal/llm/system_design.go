package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type SystemDesignRequest struct{ RecentSubjects []string }

type SystemDesignLesson struct {
	Subject      string   `json:"subject"`
	Opening      string   `json:"opening"`
	Content      string   `json:"content"`
	Mechanics    string   `json:"mechanics"`
	Tradeoffs    []string `json:"tradeoffs"`
	FailureModes []string `json:"failure_modes"`
	Example      string   `json:"example"`
	Takeaways    []string `json:"takeaways"`
}

func (c *Client) GenerateSystemDesign(ctx context.Context, request SystemDesignRequest) (SystemDesignLesson, error) {
	history := strings.Join(request.RecentSubjects, ", ")
	prompt := fmt.Sprintf("Crie uma lição autocontida de System Design em português sobre um conceito concreto. Use conceitos como distribuição, replicação, particionamento, consistência, consenso, filas, cache, storage, redes, idempotência ou observabilidade. Explique funcionamento interno, mecanismos, trade-offs, modos de falha e um exemplo. Não faça coaching de entrevista, gestão ou conteúdo superficial. Retorne JSON com subject, opening, content, mechanics, tradeoffs, failure_modes, example e takeaways (3 a 5). Evite repetir estes assuntos: %s", safePromptField(history))
	var lesson SystemDesignLesson
	if err := c.completeJSON(ctx, "Você é um professor de System Design. Responda somente JSON válido.", prompt, &lesson); err != nil {
		return SystemDesignLesson{}, err
	}
	if strings.TrimSpace(lesson.Subject) == "" || strings.TrimSpace(lesson.Content) == "" || strings.TrimSpace(lesson.Mechanics) == "" || strings.TrimSpace(lesson.Example) == "" {
		return SystemDesignLesson{}, fmt.Errorf("system design lesson is incomplete")
	}
	if len(lesson.Tradeoffs) < 1 || len(lesson.FailureModes) < 1 || len(lesson.Takeaways) < 3 || len(lesson.Takeaways) > 5 {
		return SystemDesignLesson{}, fmt.Errorf("system design lesson sections are incomplete")
	}
	return lesson, nil
}

func (c *Client) completeJSON(ctx context.Context, system, user string, target any) error {
	body, err := json.Marshal(completionRequest{Model: c.model, Messages: []promptMessage{{Role: "system", Content: system}, {Role: "user", Content: user}}, Temperature: 0.5, MaxTokens: maxCompletionTokens})
	if err != nil {
		return fmt.Errorf("encode completion request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create completion request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call Zenifra AI: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Zenifra AI returned status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read completion response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return fmt.Errorf("completion response exceeds size limit")
	}
	var envelope completionResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode completion envelope: %w", err)
	}
	if len(envelope.Choices) == 0 {
		return fmt.Errorf("completion returned no choices")
	}
	if err := json.Unmarshal([]byte(stripCodeFence(envelope.Choices[0].Message.Content)), target); err != nil {
		return fmt.Errorf("decode completion JSON: %w", err)
	}
	return nil
}
