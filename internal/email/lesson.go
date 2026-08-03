package email

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/llm"
)

type lessonView struct {
	Date      string
	Slot      string
	Title     string
	Topic     string
	Subject   string
	Kind      string
	Opening   string
	Content   string
	Answer    string
	Takeaways []string
}

const lessonTextTemplate = `Lição de aprendizado — {{.Date}}
Domínio: {{.Topic}}

{{.Subject}}
{{.Title}}

{{.Opening}}

{{.Content}}
{{if .Answer}}
Resposta comentada:
{{.Answer}}
{{end}}
Pontos-chave:
{{range .Takeaways}}- {{.}}
{{end}}`

const lessonHTMLTemplate = `<!doctype html>
<html lang="pt-BR"><body style="font-family:Arial,sans-serif;line-height:1.6;color:#202124;max-width:760px;margin:auto">
<h1>Lição de aprendizado — {{.Date}}</h1>
<p><strong>Domínio:</strong> {{.Topic}}</p>
<h2>{{.Subject}}</h2>
<p><strong>Formato:</strong> {{.Kind}}</p>
<h3>{{.Title}}</h3>
<p>{{.Opening}}</p>
<p style="white-space:pre-line">{{.Content}}</p>
{{if .Answer}}<h3>Resposta comentada</h3><p style="white-space:pre-line">{{.Answer}}</p>{{end}}
<h3>Pontos-chave</h3><ul>{{range .Takeaways}}<li>{{.}}</li>{{end}}</ul>
</body></html>`

func RenderLesson(lesson llm.Lesson, date time.Time, slot string) (Message, error) {
	if strings.TrimSpace(lesson.Title) == "" || strings.TrimSpace(lesson.Topic) == "" || strings.TrimSpace(lesson.Subject) == "" || strings.TrimSpace(lesson.Opening) == "" || strings.TrimSpace(lesson.Content) == "" {
		return Message{}, fmt.Errorf("lesson title, topic, subject, opening and content are required")
	}
	if lesson.Kind != "question" && lesson.Kind != "text" {
		return Message{}, fmt.Errorf("lesson kind must be question or text")
	}
	if lesson.Kind == "question" && strings.TrimSpace(lesson.Answer) == "" {
		return Message{}, fmt.Errorf("question lesson answer is required")
	}
	if len(lesson.Takeaways) < 3 {
		return Message{}, fmt.Errorf("lesson needs at least three takeaways")
	}
	view := lessonView{
		Date: date.Format("02/01/2006"), Slot: strings.TrimSpace(slot), Title: lesson.Title,
		Topic: lesson.Topic, Subject: lesson.Subject, Kind: lesson.Kind, Opening: lesson.Opening, Content: lesson.Content,
		Answer: lesson.Answer, Takeaways: lesson.Takeaways,
	}
	textTpl, err := texttemplate.New("lesson").Parse(lessonTextTemplate)
	if err != nil {
		return Message{}, fmt.Errorf("parse lesson text template: %w", err)
	}
	var textBody bytes.Buffer
	if err := textTpl.Execute(&textBody, view); err != nil {
		return Message{}, fmt.Errorf("render lesson text: %w", err)
	}
	htmlTpl, err := htmltemplate.New("lesson").Parse(lessonHTMLTemplate)
	if err != nil {
		return Message{}, fmt.Errorf("parse lesson HTML template: %w", err)
	}
	var htmlBody bytes.Buffer
	if err := htmlTpl.Execute(&htmlBody, view); err != nil {
		return Message{}, fmt.Errorf("render lesson HTML: %w", err)
	}
	return Message{
		Subject:  "Lição de aprendizado — " + lesson.Subject + " — " + lesson.Topic + " — " + view.Date,
		TextBody: strings.TrimSpace(textBody.String()),
		HTMLBody: htmlBody.String(),
	}, nil
}
