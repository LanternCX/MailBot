package mailbot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Reader failures must not stop durable mail collection or Telegram delivery.
func (s *Service) readerLoop(ctx context.Context) {
	for ctx.Err() == nil {
		server := &http.Server{Addr: s.config.Reader.Listen, Handler: s.readerHandler(), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
		stop := context.AfterFunc(ctx, func() { _ = server.Close() })
		err := server.ListenAndServe()
		stop()
		if ctx.Err() != nil {
			return
		}
		slog.Error("mail reader unavailable; retrying", "error", err)
		if !wait(ctx, 5*time.Second) {
			return
		}
	}
}

// Tokens are stored independently of email IDs so predictable IMAP identifiers
// never grant access to private mail. Existing tokens survive retries and restarts.
func (s *Store) readingToken(id string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO read_links(email,token) VALUES(?,?)`, id, hex.EncodeToString(b)); err != nil {
		return "", err
	}
	var token string
	err := s.db.QueryRow(`SELECT token FROM read_links WHERE email=?`, id).Scan(&token)
	return token, err
}

const readerCSS = `
html{color-scheme:light;background:#fff}
body{margin:0;color:#202124;font:16px/1.65 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif}
main{max-width:860px;margin:0 auto;padding:36px 28px 24px;box-sizing:border-box}
header{border-bottom:1px solid #eee;padding-bottom:20px;margin-bottom:20px}
h1{font-size:24px;font-weight:600;line-height:1.4;letter-spacing:-.02em;overflow-wrap:anywhere;margin:0 0 12px}
.sender{font-size:14px;overflow-wrap:anywhere}
.meta{font-size:12px;color:#777;overflow-wrap:anywhere;margin-top:4px}
pre{white-space:pre-wrap;overflow-wrap:anywhere;font:inherit;margin:0}
iframe{display:block;width:100%;height:75vh;min-height:80px;border:0}
a{color:#555;text-underline-offset:3px}
a:hover{color:#111}
a:focus-visible{outline:2px solid #555;outline-offset:4px}
footer{border-top:1px solid #eee;font-size:11px;color:#999;margin-top:32px;padding-top:12px}
.reading-tools{display:flex;align-items:center;gap:16px;margin:0 0 24px;font-size:14px}.reading-tools form{margin:0}button{font:inherit;background:#fff;border:1px solid #ddd;border-radius:6px;padding:6px 12px;color:#333;cursor:pointer}button:disabled{color:#888;cursor:wait}.translation-note{color:#777;font-size:12px;margin:0 0 16px}.translation{font:inherit;line-height:1.8}.reading-error{color:#994c25;font-size:14px;margin-bottom:20px}
@media(max-width:600px){main{padding:20px 16px}header{padding-bottom:16px;margin-bottom:16px}h1{font-size:20px;line-height:1.45}}
`

// Only this exact app script may execute. The mail frame allows same-origin
// measurement but never scripts, so long messages use the page's normal scroll.
const readerResize = `const frame=document.querySelector('iframe');if(frame){frame.addEventListener('load',()=>{const doc=frame.contentDocument;frame.style.height='80px';const resize=()=>{const height=Math.max(80,doc.body.scrollHeight,doc.documentElement.scrollHeight)+'px';if(frame.style.height!==height)frame.style.height=height};new ResizeObserver(resize).observe(doc.body);resize()})}document.querySelectorAll('.reading-tools form').forEach(form=>form.addEventListener('submit',()=>{const button=form.querySelector('button');button.disabled=true;button.textContent=button.dataset.loading}));`

var readerScriptHash = func() string {
	sum := sha256.Sum256([]byte(readerResize))
	return base64.StdEncoding.EncodeToString(sum[:])
}()

var readerPage = template.Must(template.New("reader").Parse(`<!doctype html><html lang="{{.UILanguage}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.Subject}} · MailBot</title><style>` + readerCSS + `</style></head><body><main><header><h1>{{.Subject}}</h1><div class="sender">{{.Sender}}</div><div class="meta">{{.Account}}{{if not .Date.IsZero}} · {{.Date.Format "2006-01-02 15:04"}}{{end}}</div></header>{{if or .AI .Translated}}<nav class="reading-tools">{{if .Translated}}<a href="{{.OriginalURL}}">{{.OriginalLabel}}</a>{{else}}<form method="post" action="{{.TranslateURL}}"><button type="submit" data-loading="{{.LoadingLabel}}">{{.TranslateLabel}}</button></form>{{end}}</nav>{{end}}{{if .Error}}<p class="reading-error">{{.Error}}</p>{{end}}{{if .Translated}}<p class="translation-note">{{.TranslationNote}}</p><pre class="translation">{{.Translation}}</pre>{{else}}{{if .HTMLBody}}<iframe title="{{.BodyLabel}}" sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox" referrerpolicy="no-referrer" src="{{.FrameURL}}"></iframe>{{else}}<pre>{{.Body}}</pre>{{end}}{{end}}<footer>MailBot · {{.PrivateLabel}}</footer></main><script>` + readerResize + `</script></body></html>`))

type readerView struct {
	UILanguage, OriginalLabel, TranslateLabel, LoadingLabel, TranslationNote, BodyLabel, PrivateLabel string
	Email
	FrameURL, OriginalURL, TranslateURL, LanguageLabel, Translation, Error string
	AI, Translated                                                         bool
}

func (s *Service) readerHandler() http.Handler {
	client := newImageClient(s.config.Proxy)
	downloads := make(chan struct{}, 4)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		// Preserve the Origin of same-site translation forms without leaking
		// the private mail URL when navigating to another site.
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'sha256-"+readerScriptHash+"'; style-src 'unsafe-inline'; frame-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		path := strings.Split(strings.TrimPrefix(r.URL.Path, "/read/"), "/")
		token := path[0]
		frame := len(path) == 2 && path[1] == "body"
		translation := len(path) == 2 && path[1] == "translation"
		image := len(path) == 3 && path[1] == "image"
		remote := len(path) == 3 && path[1] == "remote"
		var id string
		if !strings.HasPrefix(r.URL.Path, "/read/") || len(token) != 64 || (len(path) != 1 && !frame && !image && !remote && !translation) {
			http.Error(w, "邮件不存在或已过期。", http.StatusNotFound)
			return
		}
		if err := s.store.db.QueryRow(`SELECT e.id FROM read_links l JOIN emails e ON e.id=l.email WHERE l.token=?`, token).Scan(&id); err != nil {
			http.Error(w, "邮件不存在或已过期。", http.StatusNotFound)
			return
		}
		e, err := s.store.Email(id)
		if err != nil {
			http.Error(w, "暂时无法读取邮件，请稍后再试。", http.StatusServiceUnavailable)
			return
		}
		if r.Method == http.MethodPost && !translation {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		view := readerView{Email: e, FrameURL: "/read/" + token + "/body", OriginalURL: "/read/" + token, TranslateURL: "/read/" + token + "/translation", AI: s.config.AI.Enabled, LanguageLabel: readingLanguageLabel(s.readingLanguage())}
		view.UILanguage = s.uiLanguage()
		view.OriginalLabel, view.LoadingLabel = s.tr("original"), s.tr("translating")
		view.TranslateLabel = s.tr("translate", view.LanguageLabel)
		view.TranslationNote = s.tr("note", view.LanguageLabel)
		view.BodyLabel, view.PrivateLabel = s.tr("body"), s.tr("private")
		if translation {
			s.serveTranslation(w, r, e, view)
			return
		}
		if remote {
			raw := remoteImageSource(e, path[2])
			if raw == "" {
				http.NotFound(w, r)
				return
			}
			if r.Method == http.MethodHead {
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
			defer cancel()
			select {
			case downloads <- struct{}{}:
				defer func() { <-downloads }()
			case <-ctx.Done():
				http.Error(w, "图片暂时无法加载。", http.StatusServiceUnavailable)
				return
			}
			data, kind, err := s.store.remoteImage(ctx, client, raw)
			if err != nil {
				http.Error(w, "图片暂时无法加载。", http.StatusBadGateway)
				return
			}
			w.Header().Set("Content-Type", kind)
			_, _ = w.Write(data)
			return
		}
		if image {
			i, err := strconv.Atoi(path[2])
			if err != nil || i < 0 || i >= len(e.Images) || !safeImageType(e.Images[i].Type) {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", e.Images[i].Type)
			if r.Method != http.MethodHead {
				_, _ = w.Write(e.Images[i].Data)
			}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodHead {
			return
		}
		if frame {
			w.Header().Set("Referrer-Policy", "no-referrer")
			// A separate response avoids placing untrusted markup in the app page.
			w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox allow-same-origin allow-popups allow-popups-to-escape-sandbox; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
			fmt.Fprint(w, renderMailHTML(e, token, true))
			return
		}
		_ = readerPage.Execute(w, view)
	})
}

func readingCard(e Email, link string) Card {
	return readingPreviewCard(e, link, e.Body, 180)
}

func readingPreviewCard(e Email, link, preview string, limit int, languages ...string) Card {
	language := "zh"
	if len(languages) > 0 {
		language = languages[0]
	}
	preview = strings.Join(strings.Fields(preview), " ")
	text := "📬 <b>" + template.HTMLEscapeString(truncate(e.Subject, 180)) + "</b>\n" + template.HTMLEscapeString(truncate(e.Sender, 160)) + "\n" + template.HTMLEscapeString(truncate(e.Account, 80))
	if !e.Date.IsZero() {
		text += " · " + e.Date.Format("01-02 15:04")
	}
	code := verificationCode(e)
	if code != "" {
		text += "\n\n" + uiString(language, "code") + ": <code>" + template.HTMLEscapeString(code) + "</code>"
	}
	text += "\n\n" + template.HTMLEscapeString(truncate(preview, limit))
	if len([]rune(preview)) > limit {
		text += "…"
	}
	buttons := []button{{Text: uiString(language, "body"), URL: link}}
	if code != "" {
		buttons = append(buttons, button{Text: uiString(language, "copy"), CopyText: &copyText{Text: code}})
	}
	return Card{Text: text, HTML: true, DisablePreview: true, Buttons: [][]button{buttons}}
}
