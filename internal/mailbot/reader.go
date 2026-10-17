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
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
)

// Reader failures must not stop durable mail collection or Telegram delivery.
func (s *Service) readerLoop(ctx context.Context) {
	for ctx.Err() == nil {
		server := &http.Server{Addr: s.config.Reader.Listen, Handler: s.readerHandler(), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
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

var readerPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AddTargetBlankToFullyQualifiedLinks(true)
	// Email formatting is useful, but arbitrary CSS and resource URLs are not.
	p.AllowStyles("color", "background-color", "font-size", "font-weight", "font-style", "text-align", "text-decoration", "line-height", "font-family", "padding", "margin", "border", "border-collapse", "width", "max-width").Globally()
	return p
}()

const readerCSS = `html{color-scheme:light}body{margin:0;background:#f5f3ed;color:#242c28;font:17px/1.65 Georgia,"Noto Serif",serif}main{max-width:880px;margin:40px auto;padding:32px;background:#fff;border-top:4px solid #34725a;box-sizing:border-box}header{border-bottom:1px solid #e1e5df;margin-bottom:24px;padding-bottom:24px}small{font:12px/1.5 sans-serif;letter-spacing:.14em;color:#34725a}h1{font-size:clamp(24px,5vw,36px);line-height:1.3;overflow-wrap:anywhere;margin:16px 0}p{overflow-wrap:anywhere} .meta{font:14px/1.8 sans-serif;color:#59675e;overflow-wrap:anywhere}pre{white-space:pre-wrap;overflow-wrap:anywhere;font:inherit}iframe{display:block;width:100%;height:75vh;min-height:360px;border:0}footer{font:12px/1.7 sans-serif;color:#67746c;margin-top:24px}@media(max-width:600px){main{margin:0;padding:24px 16px;min-height:100vh}}`

// Only this exact app script may execute. The mail frame allows same-origin
// measurement but never scripts, so long messages use the page's normal scroll.
const readerResize = `const frame=document.querySelector('iframe');if(frame){frame.addEventListener('load',()=>{const body=frame.contentDocument.body;const resize=()=>{frame.style.height=Math.max(80,body.scrollHeight+24)+'px'};new ResizeObserver(resize).observe(body);resize()})}`

var readerScriptHash = func() string {
	sum := sha256.Sum256([]byte(readerResize))
	return base64.StdEncoding.EncodeToString(sum[:])
}()

var readerPage = template.Must(template.New("reader").Parse(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.Subject}} · MailBot</title><style>` + readerCSS + `iframe{min-height:80px}</style></head><body><main><header><small>MAILBOT / 邮件正文</small><h1>{{.Subject}}</h1><div class="meta">{{.Sender}}<br>{{.Account}} · {{.Date.Format "2006-01-02 15:04"}}</div></header>{{if .HTMLBody}}<iframe title="邮件正文" sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox" referrerpolicy="no-referrer" src="{{.FrameURL}}"></iframe>{{else}}<pre>{{.Body}}</pre>{{end}}<footer>外部图片与附件不加载。此链接可直接访问邮件，请勿转发给无关人员。</footer></main><script>` + readerResize + `</script></body></html>`))

func (s *Service) readerHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'sha256-"+readerScriptHash+"'; style-src 'unsafe-inline'; frame-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/read/")
		frame := strings.HasSuffix(path, "/body")
		token := strings.TrimSuffix(path, "/body")
		var id string
		if !strings.HasPrefix(r.URL.Path, "/read/") || len(token) != 64 {
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodHead {
			return
		}
		if frame {
			// A separate response avoids placing untrusted markup in the app page.
			w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox allow-same-origin allow-popups allow-popups-to-escape-sandbox; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
			fmt.Fprint(w, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><style>body{display:flow-root;margin:0;font:16px/1.6 sans-serif;overflow-wrap:anywhere}table{max-width:100%}pre{white-space:pre-wrap}img{display:none}</style></head><body>`)
			fmt.Fprint(w, readerPolicy.Sanitize(e.HTMLBody))
			fmt.Fprint(w, `</body></html>`)
			return
		}
		_ = readerPage.Execute(w, struct {
			Email
			FrameURL string
		}{e, "/read/" + token + "/body"})
	})
}

var verificationDigits = regexp.MustCompile(`(?i)(?:验证码|verification\s*code|security\s*code|one.time\s*code|code)[^0-9\n]{0,40}\b([0-9]{4,8})\b`)

func readingCard(e Email, link string) Card {
	preview := strings.Join(strings.Fields(e.Body), " ")
	text := "📬 <b>" + template.HTMLEscapeString(truncate(e.Subject, 180)) + "</b>\n" + template.HTMLEscapeString(truncate(e.Sender, 160)) + "\n" + template.HTMLEscapeString(truncate(e.Account, 80))
	if !e.Date.IsZero() {
		text += " · " + e.Date.Format("01-02 15:04")
	}
	if verificationMail(e) {
		if match := verificationDigits.FindStringSubmatch(truncate(e.Body, 1500)); len(match) > 1 {
			text += "\n\n验证码：<code>" + match[1] + "</code>"
		}
	}
	text += "\n\n" + template.HTMLEscapeString(truncate(preview, 180))
	if len([]rune(preview)) > 180 {
		text += "…"
	}
	return Card{Text: text, HTML: true, DisablePreview: true, Buttons: [][]button{{{Text: "查看正文", URL: link}}}}
}
