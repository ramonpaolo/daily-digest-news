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

func RenderEngineeringBlogs(digest llm.BlogDigest, date time.Time) (Message, error) {
	if strings.TrimSpace(digest.Intro) == "" || len(digest.Items) < 1 || len(digest.Items) > 3 {
		return Message{}, fmt.Errorf("engineering blog digest is incomplete")
	}
	for _, item := range digest.Items {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.URL) == "" || strings.TrimSpace(item.Summary) == "" {
			return Message{}, fmt.Errorf("engineering blog item is incomplete")
		}
	}
	view := struct {
		Date, Intro string
		Items       []llm.BlogDigestItem
	}{date.Format("02/01/2006"), digest.Intro, digest.Items}
	textTpl, err := template.New("blogs").Parse(`Engineering Blogs — semana de {{.Date}}

{{.Intro}}
{{range .Items}}
{{.SourceName}} — {{.Title}}
{{.Summary}}

Por que importa: {{.WhyItMatters}}
Ideias técnicas:
{{range .KeyIdeas}}- {{.}}
{{end}}Trade-offs: {{.Tradeoffs}}
Link: {{.URL}}
{{end}}`)
	if err != nil {
		return Message{}, err
	}
	var text bytes.Buffer
	if err := textTpl.Execute(&text, view); err != nil {
		return Message{}, err
	}
	htmlTpl, err := htmltemplate.New("blogs").Parse(`<html lang="pt-BR"><body style="font-family:Arial,sans-serif;line-height:1.6;max-width:760px;margin:auto"><h1>Engineering Blogs — semana de {{.Date}}</h1><p>{{.Intro}}</p>{{range .Items}}<article><h2><a href="{{.URL}}">{{.Title}}</a></h2><p><strong>Fonte:</strong> {{.SourceName}}</p><p>{{.Summary}}</p><p><strong>Por que importa:</strong> {{.WhyItMatters}}</p><h3>Ideias técnicas</h3><ul>{{range .KeyIdeas}}<li>{{.}}</li>{{end}}</ul><p><strong>Trade-offs:</strong> {{.Tradeoffs}}</p></article>{{end}}</body></html>`)
	if err != nil {
		return Message{}, err
	}
	var html bytes.Buffer
	if err := htmlTpl.Execute(&html, view); err != nil {
		return Message{}, err
	}
	return Message{Subject: "Engineering Blogs — semana de " + view.Date, TextBody: strings.TrimSpace(text.String()), HTMLBody: html.String()}, nil
}
