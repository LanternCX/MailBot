package mailbot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReaderPreservesMailAcrossRestartAndExpiresAfterCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail.sqlite3")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var e Email
	e.ID = "html"
	raw := "From: sender@example.com\r\nContent-Type: multipart/alternative; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\nFull plain body\r\n--x\r\nContent-Type: text/html\r\n\r\n<table><tr><td style=\"color:red\">Full HTML body</td></tr></table><script>alert(1)</script><img src=\"https://tracker.example/pixel\" onerror=\"alert(1)\"><form action=\"https://bad.example\"><input></form><a href=\"javascript:alert(1)\">bad link</a>\r\n--x--\r\n"
	if err := parseEmail(strings.NewReader(raw), &e); err != nil {
		t.Fatal(err)
	}
	if e.Body != "Full plain body" || !strings.Contains(e.HTMLBody, "Full HTML body") {
		t.Fatalf("lost alternatives: %+v", e)
	}
	legacy := Email{ID: "legacy", Subject: "<script>title</script>", Body: "Full legacy <body>"}
	if err := store.Commit("a", "INBOX", 1, 1, []Email{e, legacy}, []string{"t"}); err != nil {
		t.Fatal(err)
	}
	token, err := store.readingToken(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacyToken, err := store.readingToken(legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := &Service{store: store}
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		service.readerHandler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	w := request("/read/" + token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "iframe") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("page: %d %s", w.Code, w.Body)
	}
	w = request("/read/" + token + "/body")
	for _, forbidden := range []string{"<script", "onerror", "<form", "<input", "javascript:"} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("unsafe markup %q: %s", forbidden, w.Body)
		}
	}
	if !strings.Contains(w.Body.String(), "<table>") || !strings.Contains(w.Body.String(), "Full HTML body") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("missing content or isolation: %s", w.Body)
	}
	w = request("/read/" + legacyToken)
	if !strings.Contains(w.Body.String(), "Full legacy &lt;body&gt;") || strings.Contains(w.Body.String(), "<script>title") {
		t.Fatal("legacy text was not escaped")
	}
	for _, path := range []string{"/", "/read/html", "/read/" + strings.Repeat("0", 64)} {
		if request(path).Code != 404 {
			t.Fatal("private mail exposed")
		}
	}
	if _, err := store.db.Exec(`UPDATE emails SET created=?`, time.Now().Add(-31*24*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if err := store.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if request("/read/"+token).Code != 200 {
		t.Fatal("pending email expired")
	}
	if _, err := store.db.Exec(`UPDATE deliveries SET done=1`); err != nil {
		t.Fatal(err)
	}
	if err := store.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if request("/read/"+token).Code != 404 {
		t.Fatal("cleaned email still readable")
	}
}

func TestReaderDeliveryIsOneShortCardWithoutAI(t *testing.T) {
	service := &Service{store: testStore(t), config: Config{Reader: ReaderConfig{PublicURL: "https://mail.example.com"}, AI: AIConfig{Enabled: true, Mode: "agent"}}}
	var payload map[string]json.RawMessage
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer api.Close()
	tg := &Telegram{config: TelegramConfig{APIBase: api.URL, Token: "test", ChatID: "1"}, client: api.Client()}
	e := Email{ID: "mail", Subject: "Verification code", Body: "Your verification code is 123456. " + strings.Repeat("long mail ", 2000)}
	if err := service.store.Commit("a", "INBOX", 1, 1, []Email{e}, []string{"t"}); err != nil {
		t.Fatal(err)
	}
	d, err := service.store.Next("t")
	if err != nil || d == nil {
		t.Fatalf("queue: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := service.deliver(ctx, tg, d); err != nil {
		t.Fatal(err)
	}
	var text string
	json.Unmarshal(payload["text"], &text)
	if calls != 1 || len([]rune(text)) > 1000 || !strings.Contains(text, "<code>123456</code>") {
		t.Fatalf("unexpected notification: %d %s", calls, text)
	}
	var markup struct {
		Rows [][]button `json:"inline_keyboard"`
	}
	if err := json.Unmarshal(payload["reply_markup"], &markup); err != nil {
		t.Fatal(err)
	}
	link := markup.Rows[0][0].URL
	if !strings.HasPrefix(link, "https://mail.example.com/read/") {
		t.Fatal(link)
	}
	if string(payload["disable_web_page_preview"]) != "true" {
		t.Fatal("notification may fetch external link previews")
	}
	token, err := service.store.readingToken(e.ID)
	if err != nil || !strings.HasSuffix(link, token) {
		t.Fatal("link changed on retry")
	}
	// Ordinary long mail must bypass automatic analysis just like urgent mail.
	e.ID = "ordinary"
	e.Subject = "Newsletter"
	e.Body = strings.Repeat("Newsletter content. ", 2000)
	if err := service.store.Commit("a", "INBOX", 1, 2, []Email{e}, []string{"ordinary"}); err != nil {
		t.Fatal(err)
	}
	d, err = service.store.Next("ordinary")
	if err != nil || d == nil {
		t.Fatalf("queue: %v", err)
	}
	if err := service.deliver(ctx, tg, d); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(payload["text"], &text)
	if calls != 2 || len([]rune(text)) > 1000 || strings.Contains(text, "AI Summary") {
		t.Fatal("ordinary mail did not use a short card")
	}
}

func TestReaderConfigurationRequiresHTTPSOriginAndListener(t *testing.T) {
	for _, c := range []ReaderConfig{{PublicURL: "http://example.com", Listen: "127.0.0.1:8080"}, {PublicURL: "https://example.com/path", Listen: "127.0.0.1:8080"}, {PublicURL: "https://example.com"}} {
		if c.validate() == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
	if err := (ReaderConfig{PublicURL: "https://example.com", Listen: "127.0.0.1:8080"}).validate(); err != nil {
		t.Fatal(err)
	}
}
