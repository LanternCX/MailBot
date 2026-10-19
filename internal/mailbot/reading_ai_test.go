package mailbot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func readingAIService(t *testing.T, handler http.HandlerFunc) *Service {
	t.Helper()
	api := httptest.NewServer(handler)
	t.Cleanup(api.Close)
	n, err := NewNetwork(nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	return &Service{store: testStore(t), network: n, config: Config{AI: AIConfig{Enabled: true, BaseURL: api.URL, Language: "zh"}, Reader: ReaderConfig{PublicURL: "https://mail.example.com"}}}
}

func aiFixtureReply(w http.ResponseWriter, a any) {
	data, _ := json.Marshal(a)
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(data)}}}})
}

func TestReadingSummaryIsAutomaticCachedAndKeepsCodeFast(t *testing.T) {
	var calls atomic.Int32
	s := readingAIService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if !strings.Contains(req.Messages[0].Content, "1-3 sentences in zh") || strings.Contains(req.Messages[0].Content, "User Preferences") {
			t.Error("wrong summary instructions")
		}
		aiFixtureReply(w, Analysis{Summary: "这是本月账单，请于月底前付款。", SourceLanguage: "en"})
	})
	e := Email{ID: "summary", Subject: "Invoice", Body: "Your monthly bill is attached."}
	if err := s.store.Commit("a", "INBOX", 1, 1, []Email{e}, []string{"t"}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"raw", "hybrid", "agent"} {
		s.config.AI.Mode = mode
		card := s.notificationCard(context.Background(), e, "https://mail.example.com/read/token")
		if !strings.Contains(card.Text, "本月账单") || strings.Contains(card.Text, e.Body) || card.Buttons[0][0].Text != "查看正文" {
			t.Fatalf("mode %s: %+v", mode, card)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("summary was charged more than once")
	}
	e.Subject = "Your verification code"
	e.Body = "Your verification code is 482731"
	card := s.notificationCard(context.Background(), e, "https://mail.example.com/read/token")
	if calls.Load() != 1 || !strings.Contains(card.Text, "<code>482731</code>") || card.Buttons[0][1].CopyText.Text != "482731" {
		t.Fatal("verification code blocked by AI or lost")
	}
}

func TestReaderTranslationIsExplicitCompleteCachedEscapedAndLanguageSpecific(t *testing.T) {
	var calls atomic.Int32
	var supplied strings.Builder
	s := readingAIService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct{ Messages []struct{ Content string } }
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Messages[0].Content, "Translate the ENTIRE") {
			t.Error("translation must not request a summary")
		}
		parts := strings.SplitN(req.Messages[1].Content, "Body:\n", 2)
		supplied.WriteString(parts[1])
		aiFixtureReply(w, map[string]any{"translation": "译文 <script>alert(1)</script>", "source_language": "en"})
	})
	e := Email{ID: "translation", Subject: "Full letter", Body: strings.Repeat("Complete paragraph.\n", 400) + "FINAL PARAGRAPH"}
	if err := s.store.Commit("a", "INBOX", 1, 1, []Email{e}, []string{"t"}); err != nil {
		t.Fatal(err)
	}
	token, _ := s.store.readingToken(e.ID)
	path := "/read/" + token + "/translation"
	h := s.readerHandler()
	get := func(method, path, origin string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		h.ServeHTTP(w, r)
		return w
	}
	if w := get("GET", path, ""); w.Code != 200 || calls.Load() != 0 || !strings.Contains(w.Body.String(), "翻译为 中文") {
		t.Fatal("GET must not make billable calls")
	}
	if w := get("GET", "/read/"+token, ""); w.Header().Get("Referrer-Policy") != "same-origin" {
		t.Fatal("translation forms need a same-origin Origin without leaking mail URLs externally")
	}
	if w := get("GET", "/read/"+token+"/body", ""); w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("mail body must not disclose its private URL")
	}
	if w := get("POST", path, "https://other.example"); w.Code != 403 || calls.Load() != 0 {
		t.Fatal("cross-origin translation accepted")
	}
	if w := get("POST", "/read/"+strings.Repeat("0", 64)+"/translation", ""); w.Code != 404 || calls.Load() != 0 {
		t.Fatal("unauthorized translation accepted")
	}
	if w := get("POST", path, ""); w.Code != 303 || supplied.String() != e.Body || calls.Load() < 2 {
		t.Fatalf("translation omitted content: status=%d calls=%d", w.Code, calls.Load())
	}
	count := calls.Load()
	for range 2 {
		w := get("GET", path, "")
		if !strings.Contains(w.Body.String(), "译文 &lt;script&gt;") || strings.Contains(w.Body.String(), "<script>alert(1)") || !strings.Contains(w.Body.String(), "查看原邮件") || calls.Load() != count {
			t.Fatal("unsafe or uncached translation")
		}
	}
	if err := s.store.Set("language", "ja"); err != nil {
		t.Fatal(err)
	}
	if w := get("GET", path, ""); !strings.Contains(w.Body.String(), "翻译为 日本語") || strings.Contains(w.Body.String(), "AI 翻译 · 日本語") {
		t.Fatal("wrong-language cache reused")
	}
	if _, err := s.store.db.Exec(`UPDATE emails SET created=0; UPDATE deliveries SET done=1`); err != nil {
		t.Fatal(err)
	}
	if err := s.store.Cleanup(); err != nil {
		t.Fatal(err)
	}
	var rows int
	_ = s.store.db.QueryRow(`SELECT COUNT(*) FROM reading_ai`).Scan(&rows)
	if rows != 0 {
		t.Fatal("AI cache outlived email retention")
	}
}

func TestReadingAIFailureFallsBackAndTranslationCanRetry(t *testing.T) {
	var calls atomic.Int32
	s := readingAIService(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			http.Error(w, "unavailable", 503)
			return
		}
		aiFixtureReply(w, map[string]any{"translation": "完整译文"})
	})
	e := Email{ID: "failure", Body: "Original full mail"}
	_ = s.store.Commit("a", "INBOX", 1, 1, []Email{e}, []string{"t"})
	if card := s.notificationCard(context.Background(), e, "https://mail.example.com/read/token"); !strings.Contains(card.Text, e.Body) {
		t.Fatal("AI outage lost original notification")
	}
	for i := 0; i < 2; i++ {
		_, err := s.readingAnalysis(context.Background(), e, "zh", "translation")
		if (err != nil) != (i == 0) {
			t.Fatalf("retry %d: %v", i, err)
		}
	}
}

func TestAISettingsOnlyAskLanguageUnlessAdvancedSelected(t *testing.T) {
	c := DefaultConfig()
	c.AI.Enabled = true
	c.AI.APIKey = "saved-secret"
	c.AI.Provider = "custom"
	c.AI.BaseURL = "https://custom.example/v1"
	c.AI.Model = "custom-model"
	input := strings.NewReader("y\n1\nn\n")
	var out strings.Builder
	term := &terminal{scanner: bufio.NewScanner(input), input: input, out: &out}
	if err := term.ai(&c); err != nil {
		t.Fatal(err)
	}
	if c.AI.APIKey != "saved-secret" || c.AI.Model != "custom-model" || c.AI.BaseURL != "https://custom.example/v1" || c.AI.Language != "zh" || c.AI.Mode != "" {
		t.Fatalf("settings lost: %s", fmt.Sprint(c.AI.Enabled, c.AI.Language))
	}
	for _, hidden := range []string{"saved-secret", "Default operation mode", "raw", "hybrid", "Model name"} {
		if strings.Contains(out.String(), hidden) {
			t.Fatalf("unexpected daily setting %q", hidden)
		}
	}
}
