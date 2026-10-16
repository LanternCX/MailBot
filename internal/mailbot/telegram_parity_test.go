package mailbot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestTelegramPreservesLegacyWebPreviewSetting(t *testing.T) {
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["disable_web_page_preview"] != false || payload["link_preview_options"] != nil {
			t.Error("legacy web preview behavior changed")
		}
		fmt.Fprint(w, `{"ok":true,"result":{}}`)
	}))
	defer api.Close()
	tg := &Telegram{config: TelegramConfig{APIBase: api.URL, Token: "fixture", ChatID: "1"}, client: api.Client()}
	if err := tg.send(context.Background(), "https://example.org", nil); err != nil {
		t.Fatal(err)
	}
	if err := tg.sendCard(context.Background(), Card{Text: `<b>Mail</b> <a href="https://example.org">Open</a>`, HTML: true}, 0); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("unexpected request count %d", calls)
	}
}

func TestHTMLMultipartPreservesEveryVisibleCharacter(t *testing.T) {
	body := strings.Repeat("\n空格 & < > 😀\t", 1600)
	card := Card{Text: "<b>Header</b>\n<code>" + escape(body) + "</code>", HTML: true}
	var joined strings.Builder
	parts := card.Parts()
	if len(parts) < 2 {
		t.Fatal("long content not split")
	}
	for _, part := range parts {
		z := html.NewTokenizer(strings.NewReader(part))
		var stack []string
		for {
			switch z.Next() {
			case html.ErrorToken:
				if z.Err() != io.EOF {
					t.Fatal(z.Err())
				}
				if len(stack) != 0 {
					t.Fatal("unclosed multipart tags")
				}
				goto nextPart
			case html.TextToken:
				joined.Write(z.Text())
			case html.StartTagToken:
				name, _ := z.TagName()
				stack = append(stack, string(name))
			case html.EndTagToken:
				name, _ := z.TagName()
				if len(stack) == 0 || stack[len(stack)-1] != string(name) {
					t.Fatal("mismatched multipart tags")
				}
				stack = stack[:len(stack)-1]
			}
		}
	nextPart:
	}
	if joined.String() != "Header\n"+body {
		t.Fatal("multipart changed visible text or whitespace")
	}
}
