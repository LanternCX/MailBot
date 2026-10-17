package mailbot

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
	"unicode"

	xhtml "golang.org/x/net/html"
)

type Card struct {
	DisablePreview bool       `json:"disable_preview,omitempty"`
	Text           string     `json:"text"`
	HTML           bool       `json:"html,omitempty"`
	Buttons        [][]button `json:"buttons,omitempty"`
}

var codePattern = regexp.MustCompile(`(?i)(验证码|verification\s*code|登录|login|sign.?in|code\s*[:：])`)

func truncate(text string, n int) string {
	r := []rune(text)
	if len(r) > n {
		return string(r[:n])
	}
	return text
}
func skipAI(body string) bool {
	return len([]rune(body)) < 100 || codePattern.MatchString(truncate(body, 300))
}

func detectLanguage(text string) string {
	total, han, kana, hangul, arabic, cyrillic, latin := 0, 0, 0, 0, 0, 0, 0
	for _, r := range []rune(truncate(text, 1000)) {
		if unicode.IsLetter(r) {
			total++
		}
		switch {
		case r >= 0x4e00 && r <= 0x9fff:
			han++
		case r >= 0x3040 && r <= 0x30ff:
			kana++
		case r >= 0xac00 && r <= 0xd7af:
			hangul++
		case r >= 0x600 && r <= 0x6ff:
			arabic++
		case r >= 0x400 && r <= 0x4ff:
			cyrillic++
		case r < 128 && unicode.IsLetter(r):
			latin++
		}
	}
	if total == 0 {
		return ""
	}
	if float64(han+kana+hangul)/float64(total) > .3 {
		if kana > han {
			return "ja"
		}
		if hangul > han {
			return "ko"
		}
		return "zh"
	}
	if float64(arabic)/float64(total) > .3 {
		return "ar"
	}
	if float64(cyrillic)/float64(total) > .3 {
		return "ru"
	}
	if float64(latin)/float64(total) > .7 {
		return "en"
	}
	return ""
}

func previewText(e Email) string {
	lines := []string{"📬 New mail", "", "📧 Account: " + e.Account, "👤 From: " + e.Sender, "📌 Subject: " + e.Subject}
	if !e.Date.IsZero() {
		lines = append(lines, "🕐 Time: "+e.Date.Format("2006-01-02 15:04"))
	}
	if e.Body != "" {
		preview := truncate(e.Body, 50)
		if len([]rune(e.Body)) > 50 {
			preview += "..."
		}
		lines = append(lines, "", "📝 Preview:\n"+preview)
	}
	if e.WebLink != "" {
		lines = append(lines, "", "🔗 Open in webmail: "+e.WebLink)
	}
	return strings.Join(lines, "\n")
}

func webAnchor(link string) string {
	if link == "" {
		return ""
	}
	return "\n\n🔗 <a href=\"" + html.EscapeString(link) + "\">Open in webmail</a>"
}
func mailButtons(e Email, mode, source, target string) [][]button {
	var row []button
	if mode == "hybrid" {
		row = append(row, button{Text: "✨ AI Summary", Data: "summ_" + e.ID})
	}
	row = append(row, button{Text: "📖 Original", Data: "orig_" + e.ID})
	if source != "" && target != "" && source != target {
		row = append(row, button{Text: "🌐 Translate", Data: "trans_" + e.ID})
	}
	return [][]button{row}
}

func hybridCard(e Email, language string) Card {
	if skipAI(e.Body) {
		return Card{Text: previewText(e)}
	}
	preview := truncate(e.Body, 50)
	if len([]rune(e.Body)) > 50 {
		preview += "…"
	}
	text := "📬 <b>New mail</b>\n📧 Account: " + html.EscapeString(e.Account) + "\n👤 From: " + html.EscapeString(e.Sender) + "\n📌 Subject: " + html.EscapeString(e.Subject)
	if !e.Date.IsZero() {
		text += "\n🕐 Time: " + e.Date.Format("2006-01-02 15:04")
	}
	text += "\n\n📝 Preview:\n" + html.EscapeString(preview) + webAnchor(e.WebLink)
	return Card{Text: text, HTML: true, Buttons: mailButtons(e, "hybrid", detectLanguage(e.Body), language)}
}

var categoryIcons = map[string]string{"verification_code": "🔑", "notification": "📢", "billing": "💰", "promotion": "📣", "personal": "✉️"}
var priorityLabels = map[int]string{1: "🔴 Urgent", 2: "🟠 High", 3: "🟡 Medium", 4: "🔵 Low", 5: "⚪ Lowest"}

func summaryCard(a Analysis) Card {
	icon := categoryIcons[a.Category]
	if icon == "" {
		icon = "📧"
	}
	priority := priorityLabels[a.Priority]
	if priority == "" {
		priority = priorityLabels[3]
	}
	text := fmt.Sprintf("🤖 <b>AI Summary</b>  %s %s  |  %s\n💡 %s", icon, html.EscapeString(a.Category), priority, html.EscapeString(a.Summary))
	if a.Code != nil && *a.Code != "" {
		text += "\n🔑 Code: <code>" + html.EscapeString(*a.Code) + "</code>"
	}
	return Card{Text: text, HTML: true}
}

func originalCard(e Email) Card {
	text := "📧 <b>Original Email</b>\n👤 From: " + html.EscapeString(e.Sender) + "\n📌 Subject: " + html.EscapeString(e.Subject)
	if !e.Date.IsZero() {
		text += "\n🕐 Date: " + e.Date.Format("2006-01-02 15:04:05")
	}
	text += "\n\n📝 <b>Content:</b>\n" + html.EscapeString(e.Body) + webAnchor(e.WebLink)
	return Card{Text: text, HTML: true}
}

func manualAnalysisCard(a Analysis) Card {
	icon := categoryIcons[a.Category]
	if icon == "" {
		icon = "📧"
	}
	priority := priorityLabels[a.Priority]
	if priority == "" {
		priority = priorityLabels[3]
	}
	text := fmt.Sprintf("🤖 <b>AI Analysis</b>\n\n%s Category: <b>%s</b>\n📊 Priority: %s\n\n💡 Summary:\n%s", icon, html.EscapeString(a.Category), priority, html.EscapeString(a.Summary))
	if a.Code != nil && *a.Code != "" {
		text += "\n\n🔑 Code: <code>" + html.EscapeString(*a.Code) + "</code>"
	}
	return Card{Text: text, HTML: true}
}

func encodeCard(c Card) string { b, _ := json.Marshal(c); return string(b) }
func decodeCard(text string) Card {
	var c Card
	if json.Unmarshal([]byte(text), &c) != nil || c.Text == "" {
		return Card{Text: text}
	}
	return c
}
func (c Card) Parts() []string {
	if c.HTML {
		return splitHTML(c.Text)
	}
	return splitText(c.Text)
}

// Split only text tokens, closing and reopening active tags at each boundary.
// Count encoded bytes conservatively so entities and UTF-16 characters stay safe.
func splitHTML(text string) []string {
	z := xhtml.NewTokenizer(strings.NewReader(text))
	var parts []string
	var out strings.Builder
	type tag struct{ name, raw string }
	var tags []tag
	flush := func() {
		if out.Len() == 0 {
			return
		}
		for i := len(tags) - 1; i >= 0; i-- {
			out.WriteString("</" + tags[i].name + ">")
		}
		parts = append(parts, out.String())
		out.Reset()
		for _, t := range tags {
			out.WriteString(t.raw)
		}
	}
	for {
		kind := z.Next()
		if kind == xhtml.ErrorToken {
			if z.Err() != io.EOF {
				return splitText(html.UnescapeString(text))
			}
			break
		}
		switch kind {
		case xhtml.TextToken:
			for _, r := range string(z.Text()) {
				encoded := html.EscapeString(string(r))
				if out.Len()+len(encoded)+128 > 3800 {
					flush()
				}
				out.WriteString(encoded)
			}
		case xhtml.StartTagToken:
			raw := string(z.Raw())
			name, _ := z.TagName()
			if out.Len()+len(raw)+128 > 3800 {
				flush()
			}
			out.WriteString(raw)
			tags = append(tags, tag{string(name), raw})
		case xhtml.EndTagToken:
			out.Write(z.Raw())
			if len(tags) > 0 {
				tags = tags[:len(tags)-1]
			}
		}
	}
	if out.Len() > 0 {
		parts = append(parts, out.String())
	}
	if len(parts) == 0 {
		return []string{"(Empty email)"}
	}
	return parts
}
