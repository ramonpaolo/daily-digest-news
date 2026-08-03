package email

import (
	"bytes"
	"fmt"
	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	htmltemplate "html/template"
	"strings"
	"text/template"
	"time"
)

const systemDesignText = `System Design — {{.Date}}

{{.Subject}}
{{.Opening}}

{{.Content}}

Funcionamento interno:
{{.Mechanics}}

Trade-offs:
{{range .Tradeoffs}}- {{.}}
{{end}}
Modos de falha:
{{range .FailureModes}}- {{.}}
{{end}}
Exemplo:
{{.Example}}

Pontos-chave:
{{range .Takeaways}}- {{.}}
{{end}}`

const systemDesignHTML = `<html lang="pt-BR"><body style="font-family:Arial,sans-serif;line-height:1.6;max-width:760px;margin:auto"><h1>System Design — {{.Date}}</h1><h2>{{.Subject}}</h2><p>{{.Opening}}</p><p style="white-space:pre-line">{{.Content}}</p><h3>Funcionamento interno</h3><p style="white-space:pre-line">{{.Mechanics}}</p><h3>Trade-offs</h3><ul>{{range .Tradeoffs}}<li>{{.}}</li>{{end}}</ul><h3>Modos de falha</h3><ul>{{range .FailureModes}}<li>{{.}}</li>{{end}}</ul><h3>Exemplo</h3><p style="white-space:pre-line">{{.Example}}</p><h3>Pontos-chave</h3><ul>{{range .Takeaways}}<li>{{.}}</li>{{end}}</ul></body></html>`

func RenderSystemDesign(lesson llm.SystemDesignLesson, date time.Time) (Message, error) {
	if strings.TrimSpace(lesson.Subject) == "" || strings.TrimSpace(lesson.Content) == "" || strings.TrimSpace(lesson.Mechanics) == "" || len(lesson.Takeaways) < 3 {
		return Message{}, fmt.Errorf("system design lesson is incomplete")
	}
	view := struct {
		Date, Subject, Opening, Content, Mechanics, Example string
		Tradeoffs, FailureModes, Takeaways                  []string
	}{date.Format("02/01/2006"), lesson.Subject, lesson.Opening, lesson.Content, lesson.Mechanics, lesson.Example, lesson.Tradeoffs, lesson.FailureModes, lesson.Takeaways}
	t, err := template.New("system-design").Parse(systemDesignText)
	if err != nil {
		return Message{}, err
	}
	var text bytes.Buffer
	if err := t.Execute(&text, view); err != nil {
		return Message{}, err
	}
	h, err := htmltemplate.New("system-design").Parse(systemDesignHTML)
	if err != nil {
		return Message{}, err
	}
	var html bytes.Buffer
	if err := h.Execute(&html, view); err != nil {
		return Message{}, err
	}
	return Message{Subject: "System Design — " + lesson.Subject + " — " + view.Date, TextBody: strings.TrimSpace(text.String()), HTMLBody: html.String()}, nil
}
