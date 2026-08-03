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
	ID           int
	Title        string
	Link         string
	Score        int
	Comments     int
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
{{$item.Summary}}

Por que importa: {{$item.WhyItMatters}}
Score: {{$item.Score}} | Comentários: {{$item.Comments}}
Link: {{$item.Link}}

{{end}}`

const htmlTemplate = `<!doctype html>
<html lang="pt-BR"><body style="font-family:Arial,sans-serif;line-height:1.5;color:#202124;max-width:760px;margin:auto">
<h1>Daily Digest News — {{.Date}}</h1>
<p>{{.Intro}}</p>
{{range $index, $item := .Items}}
<article style="margin:2em 0;border-top:1px solid #ddd;padding-top:1em">
  <h2>{{$index | addOne}}. <a href="{{$item.Link}}">{{$item.Title}}</a></h2>
  <p>{{$item.Summary}}</p>
  <p><strong>Por que importa:</strong> {{$item.WhyItMatters}}</p>
  <p style="color:#666;font-size:.9em">Score: {{$item.Score}} | Comentários: {{$item.Comments}} · <a href="{{$item.Link}}">Abrir artigo</a></p>
</article>
{{end}}
</body></html>`

func Render(stories []news.Story, digest llm.Digest, date time.Time) (Message, error) {
	byID := make(map[int]llm.Item, len(digest.Items))
	for _, item := range digest.Items {
		if _, exists := byID[item.StoryID]; exists {
			return Message{}, fmt.Errorf("duplicate story_id %d", item.StoryID)
		}
		byID[item.StoryID] = item
	}
	view := digestView{Date: date.Format("02/01/2006"), Intro: digest.Intro, Items: make([]itemView, 0, len(stories))}
	for _, story := range stories {
		item, ok := byID[story.ID]
		if !ok {
			return Message{}, fmt.Errorf("missing digest story_id %d", story.ID)
		}
		view.Items = append(view.Items, itemView{
			ID:           story.ID,
			Title:        story.Title,
			Link:         safeLink(story.URL, story.ID),
			Score:        story.Score,
			Comments:     story.Descendants,
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

func safeLink(raw string, storyID int) string {
	parsed, err := url.Parse(raw)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil {
		return parsed.String()
	}
	return fmt.Sprintf("https://news.ycombinator.com/item?id=%d", storyID)
}
