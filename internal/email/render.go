package email

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	"github.com/ramonpaolo/daily-digest-news/internal/news"
)

type Message struct {
	Subject  string
	TextBody string
	HTMLBody string
}

type itemView struct {
	ID           string
	SourceName   string
	Title        string
	Link         string
	Metrics      string
	Summary      string
	WhyItMatters string
}

type digestView struct {
	Date  string
	Intro string
	Items []itemView
}

const textTemplate = `Daily Digest News — {{.Date}}

{{.Intro}}

{{range $index, $item := .Items}}{{$number := add $index 1}}{{$number}}. {{$item.Title}}
Fonte: {{$item.SourceName}}
{{$item.Summary}}

Por que importa: {{$item.WhyItMatters}}
{{$item.Metrics}}
Link: {{$item.Link}}

{{end}}`

const htmlTemplate = `<!doctype html>
<html lang="pt-BR"><body style="font-family:Arial,sans-serif;line-height:1.5;color:#202124;max-width:760px;margin:auto">
<h1>Daily Digest News — {{.Date}}</h1>
<p>{{.Intro}}</p>
{{range $index, $item := .Items}}
<article style="margin:2em 0;border-top:1px solid #ddd;padding-top:1em">
  <h2>{{$index | addOne}}. <a href="{{$item.Link}}">{{$item.Title}}</a></h2>
  <p style="color:#666;font-size:.9em">Fonte: {{$item.SourceName}}</p>
  <p>{{$item.Summary}}</p>
  <p><strong>Por que importa:</strong> {{$item.WhyItMatters}}</p>
  <p style="color:#666;font-size:.9em">{{$item.Metrics}} · <a href="{{$item.Link}}">Abrir artigo</a></p>
</article>
{{end}}
</body></html>`

func Render(stories []news.Story, digest llm.Digest, date time.Time) (Message, error) {
	byID := make(map[string]llm.Item, len(digest.Items))
	for _, item := range digest.Items {
		if _, exists := byID[item.StoryID]; exists {
			return Message{}, fmt.Errorf("duplicate story_id %s", item.StoryID)
		}
		byID[item.StoryID] = item
	}
	view := digestView{Date: date.Format("02/01/2006"), Intro: digest.Intro, Items: make([]itemView, 0, len(stories))}
	for _, story := range stories {
		item, ok := byID[story.ID]
		if !ok {
			return Message{}, fmt.Errorf("missing digest story_id %s", story.ID)
		}
		link := safeLink(story.URL, story.Permalink)
		if link == "" {
			return Message{}, fmt.Errorf("story %s has no safe link", story.ID)
		}
		view.Items = append(view.Items, itemView{
			ID:           story.ID,
			SourceName:   story.SourceName,
			Title:        story.Title,
			Link:         link,
			Metrics:      formatMetrics(story.Score, story.Comments),
			Summary:      item.Summary,
			WhyItMatters: item.WhyItMatters,
		})
	}

	textTpl, err := texttemplate.New("digest").Funcs(texttemplate.FuncMap{"add": func(a, b int) int { return a + b }}).Parse(textTemplate)
	if err != nil {
		return Message{}, fmt.Errorf("parse text template: %w", err)
	}
	var textBody bytes.Buffer
	if err := textTpl.Execute(&textBody, view); err != nil {
		return Message{}, fmt.Errorf("render text body: %w", err)
	}
	htmlTpl, err := htmltemplate.New("digest").Funcs(htmltemplate.FuncMap{"addOne": func(value int) int { return value + 1 }}).Parse(htmlTemplate)
	if err != nil {
		return Message{}, fmt.Errorf("parse HTML template: %w", err)
	}
	var htmlBody bytes.Buffer
	if err := htmlTpl.Execute(&htmlBody, view); err != nil {
		return Message{}, fmt.Errorf("render HTML body: %w", err)
	}
	return Message{
		Subject:  "Daily Digest News — " + view.Date,
		TextBody: strings.TrimSpace(textBody.String()),
		HTMLBody: htmlBody.String(),
	}, nil
}

func safeLink(raw, fallback string) string {
	for _, candidate := range []string{raw, fallback} {
		parsed, err := url.Parse(candidate)
		if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil {
			return parsed.String()
		}
	}
	return ""
}

func formatMetrics(score, comments *int) string {
	parts := make([]string, 0, 2)
	if score != nil {
		parts = append(parts, fmt.Sprintf("Score: %d", *score))
	}
	if comments != nil {
		parts = append(parts, fmt.Sprintf("Comentários: %d", *comments))
	}
	return strings.Join(parts, " | ")
}
