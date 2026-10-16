package mailbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-imap/server"
)

type recordedCall struct {
	Method  string
	Payload map[string]any
}
type recorder struct {
	mu    sync.Mutex
	calls []recordedCall
}

func (r *recorder) snapshot() []recordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedCall{}, r.calls...)
}
func (r *recorder) reset() { r.mu.Lock(); r.calls = nil; r.mu.Unlock() }
func parityService(t *testing.T) (*Service, *Telegram, *recorder) {
	t.Helper()
	r := &recorder{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var payload map[string]any
		json.NewDecoder(req.Body).Decode(&payload)
		method := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
		r.mu.Lock()
		r.calls = append(r.calls, recordedCall{method, payload})
		r.mu.Unlock()
		if method == "completions" {
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"summary":"Fixture summary","category":"billing","priority":3,"translation":"翻译","source_language":"en","extracted_code":"123456"}`}}}})
			return
		}
		result := any(true)
		if method == "getUpdates" {
			result = []any{}
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	}))
	t.Cleanup(api.Close)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	c := DefaultConfig()
	c.StatePath = filepath.Join(dir, "state.sqlite3")
	c.AI = AIConfig{Enabled: true, Provider: "openai", BaseURL: api.URL, Model: "fixture", Mode: "hybrid", Language: "zh"}
	c.Notifiers = []Notifier{{Type: "telegram", Enabled: true, Telegram: &TelegramConfig{Token: "token", ChatID: "42", APIBase: api.URL, Timeout: 2}}}
	if err := SaveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	s, err := NewService(c, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.store.Close(); s.network.Close() })
	return s, s.telegrams[0], r
}
func botMessage(text string) update {
	return update{Message: &message{ID: 10, Text: text, Chat: chat{ID: "42"}}}
}
func botCallback(data string) update {
	return update{Callback: &callback{ID: "cq", Data: data, Message: &message{ID: 10, Chat: chat{ID: "42"}}}}
}
func handle(t *testing.T, s *Service, tg *Telegram, u update) chan func() {
	t.Helper()
	jobs := make(chan func(), 8)
	if err := s.handleUpdate(context.Background(), tg, u, jobs); err != nil {
		t.Fatal(err)
	}
	return jobs
}
func findCall(t *testing.T, r *recorder, method string) recordedCall {
	t.Helper()
	for _, c := range r.snapshot() {
		if c.Method == method {
			return c
		}
	}
	t.Fatalf("missing %s: %+v", method, r.snapshot())
	return recordedCall{}
}

func TestLegacyPreviewAndHybridHeuristics(t *testing.T) {
	for _, tc := range []struct {
		body, language string
		skip           bool
		detected       string
	}{
		{"hello", "en", true, "en"}, {strings.Repeat("plain newsletter ", 15), "zh", false, "en"},
		{"Verification code: 123456 " + strings.Repeat("a", 150), "zh", true, "en"},
		{strings.Repeat("这是中文邮件内容", 20), "zh", false, "zh"}, {strings.Repeat("あいうえお", 30), "en", false, "ja"},
		{strings.Repeat("안녕하세요", 30), "en", false, "ko"}, {strings.Repeat("مرحبا", 30), "en", false, "ar"}, {strings.Repeat("Привет", 30), "en", false, "ru"},
	} {
		t.Run(tc.detected+fmt.Sprint(tc.skip), func(t *testing.T) {
			if got := skipAI(tc.body); got != tc.skip {
				t.Fatalf("skip=%v", got)
			}
			if got := detectLanguage(tc.body); got != tc.detected {
				t.Fatalf("language=%q", got)
			}
			e := Email{ID: "id", Account: "Work", Subject: "Subject", Body: tc.body}
			card := hybridCard(e, tc.language)
			if tc.skip {
				if card.HTML || len(card.Buttons) > 0 {
					t.Fatal("short/code email received AI controls")
				}
			} else {
				if !card.HTML || card.Buttons[0][0].Data != "summ_id" || card.Buttons[0][1].Data != "orig_id" {
					t.Fatalf("missing hybrid controls: %+v", card)
				}
				wantTranslate := tc.detected != "" && tc.detected != tc.language
				if (len(card.Buttons[0]) == 3) != wantTranslate {
					t.Fatal("translation visibility differs")
				}
			}
			if strings.Contains(previewText(e), truncate(tc.body, 51)) && len([]rune(tc.body)) > 50 {
				t.Fatal("preview exceeds 50 characters")
			}
		})
	}
}

func TestSettingsDashboardNavigationAndJSONPersistence(t *testing.T) {
	s, tg, r := parityService(t)
	handle(t, s, tg, botMessage("/settings"))
	c := findCall(t, r, "sendMessage")
	if c.Payload["parse_mode"] != "HTML" || !strings.Contains(c.Payload["text"].(string), "MailBot Settings") {
		t.Fatal("missing settings dashboard")
	}
	for _, tc := range []struct{ data, contains string }{{"settings_lang", "Language Settings"}, {"settings_mode", "Operation Mode"}, {"settings_back", "MailBot Settings"}, {"lang_ja", "日本語"}, {"mode_agent", "agent"}} {
		r.reset()
		handle(t, s, tg, botCallback(tc.data))
		c = findCall(t, r, "editMessageText")
		if c.Payload["message_id"] != float64(10) || !strings.Contains(c.Payload["text"].(string), tc.contains) {
			t.Fatalf("wrong navigation for %s: %+v", tc.data, c)
		}
	}
	cfg, err := LoadConfig(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Language != "ja" || cfg.AI.Mode != "agent" {
		t.Fatalf("settings not in JSON: %+v", cfg.AI)
	}
	r.reset()
	handle(t, s, tg, botCallback("settings_close"))
	findCall(t, r, "deleteMessage")
}

func TestSettingsCallbacksAcknowledgeUnchangedAndFailedSaves(t *testing.T) {
	s, tg, r := parityService(t)
	// No configuration path: unchanged selections must not attempt to save.
	s.configPath = ""
	for _, data := range []string{"lang_zh", "mode_hybrid"} {
		r.reset()
		handle(t, s, tg, botCallback(data))
		calls := r.snapshot()
		if len(calls) != 1 || calls[0].Method != "answerCallbackQuery" || !strings.HasPrefix(calls[0].Payload["text"].(string), "Already") {
			t.Fatalf("unchanged setting should only acknowledge: %+v", calls)
		}
	}
	for _, data := range []string{"lang_ja", "mode_agent"} {
		r.reset()
		err := s.handleUpdate(context.Background(), tg, botCallback(data), make(chan func(), 1))
		if err == nil {
			t.Fatal("save failure was ignored")
		}
		calls := r.snapshot()
		if len(calls) != 1 || calls[0].Method != "answerCallbackQuery" || !strings.Contains(calls[0].Payload["text"].(string), "Unable to save") {
			t.Fatalf("save failure did not acknowledge callback: %+v", calls)
		}
	}
	if s.mode() != "hybrid" || s.store.Setting("language", s.config.AI.Language) != "zh" {
		t.Fatal("failed save changed active settings")
	}
}

func TestRulesConversationButtonsCancellationAndPersistence(t *testing.T) {
	s, tg, r := parityService(t)
	handle(t, s, tg, botMessage("/rules"))
	c := findCall(t, r, "sendMessage")
	if c.Payload["reply_markup"] == nil {
		t.Fatal("missing rule controls")
	}
	handle(t, s, tg, botCallback("rules_add"))
	handle(t, s, tg, botMessage("Summarize in detail"))
	handle(t, s, tg, botMessage("/rules add"))
	handle(t, s, tg, botMessage("Focus on invoices"))
	rules, err := loadRules(filepath.Join(s.configDir, "rules.md"))
	if err != nil || len(rules) != 2 {
		t.Fatalf("rules missing: %v %v", rules, err)
	}
	handle(t, s, tg, botCallback("rules_delete"))
	handle(t, s, tg, botMessage("delete number 1 please"))
	rules, _ = loadRules(filepath.Join(s.configDir, "rules.md"))
	if len(rules) != 1 || rules[0] != "Focus on invoices" {
		t.Fatalf("delete failed: %v", rules)
	}
	handle(t, s, tg, botMessage("/rules add"))
	handle(t, s, tg, botMessage("/rules"))
	handle(t, s, tg, botMessage("must not become a rule"))
	rules, _ = loadRules(filepath.Join(s.configDir, "rules.md"))
	if len(rules) != 1 {
		t.Fatal("/rules did not cancel conversation")
	}
	r.reset()
	handle(t, s, tg, botCallback("rules_close"))
	findCall(t, r, "deleteMessage")
}

func TestQueuedDeliveryPreservesLegacyModesAcrossRestart(t *testing.T) {
	for _, mode := range []string{"raw", "hybrid", "agent"} {
		t.Run(mode, func(t *testing.T) {
			s, tg, r := parityService(t)
			if err := s.store.Set("mode", mode); err != nil {
				t.Fatal(err)
			}
			e := Email{ID: emailID("a", "INBOX", 1, 7), Account: "Work", Subject: "Invoice", Sender: "sender@example.org", Body: strings.Repeat("Invoice details ", 15)}
			if err := s.store.Commit("a", "INBOX", 1, 7, []Email{e}, s.targets); err != nil {
				t.Fatal(err)
			}
			path := s.store.path
			if err := s.store.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			s.store, err = OpenStore(path)
			if err != nil {
				t.Fatal(err)
			}
			d, err := s.store.Next(tg.key)
			if err != nil || d == nil {
				t.Fatalf("pending email lost after restart: %+v %v", d, err)
			}
			if err = s.deliver(context.Background(), tg, d); err != nil {
				t.Fatal(err)
			}
			call := findCall(t, r, "sendMessage")
			text := call.Payload["text"].(string)
			aiCalls := 0
			for _, c := range r.snapshot() {
				if c.Method == "completions" {
					aiCalls++
				}
			}
			if mode == "agent" {
				if aiCalls != 1 || !strings.Contains(text, "Fixture summary") || !strings.Contains(text, "123456") {
					t.Fatalf("automatic summary missing: %q, AI calls=%d", text, aiCalls)
				}
			} else if aiCalls != 0 || !strings.Contains(text, "Invoice") || !strings.Contains(text, "sender@example.org") {
				t.Fatalf("preview or on-demand behavior differs: %q, AI calls=%d", text, aiCalls)
			}
			markup, _ := json.Marshal(call.Payload["reply_markup"])
			for _, action := range []string{"summ_", "orig_", "trans_"} {
				want := mode != "raw" && (action != "summ_" || mode == "hybrid")
				if strings.Contains(string(markup), action+e.ID) != want {
					t.Fatalf("unexpected %s button in %s: %s", action, mode, markup)
				}
			}
			// Reopening after successful delivery must retain its saved progress.
			if err = s.store.Close(); err != nil {
				t.Fatal(err)
			}
			s.store, err = OpenStore(path)
			if err != nil {
				t.Fatal(err)
			}
			d, err = s.store.Next(tg.key)
			if err != nil || d == nil {
				t.Fatalf("delivery progress unavailable: %+v %v", d, err)
			}
			r.reset()
			if err = s.deliver(context.Background(), tg, d); err != nil {
				t.Fatal(err)
			}
			if len(r.snapshot()) != 0 {
				t.Fatalf("completed message or AI request repeated after restart: %+v", r.snapshot())
			}
		})
	}
}

func TestEmailCallbacksReplyAndManualAI(t *testing.T) {
	s, tg, r := parityService(t)
	e := Email{ID: emailID("a", "INBOX", 1, 7), Subject: "Invoice", Sender: "sender@example.org", Body: strings.Repeat("Invoice text ", 15), WebLink: "https://mail.example.org/7"}
	if err := s.store.Commit("a", "INBOX", 1, 7, []Email{e}, s.targets); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"orig_", "summ_", "trans_"} {
		r.reset()
		jobs := handle(t, s, tg, botCallback(prefix+e.ID))
		if len(jobs) > 0 {
			(<-jobs)()
		}
		c := findCall(t, r, "sendMessage")
		if c.Payload["reply_to_message_id"] != float64(10) || c.Payload["parse_mode"] != "HTML" {
			t.Fatalf("callback detached from email: %+v", c)
		}
		if prefix != "orig_" {
			findCall(t, r, "sendChatAction")
		}
	}
	r.reset()
	u := botMessage("/ai")
	u.Message.Reply = &message{ID: 99, Text: "Analyze this"}
	jobs := handle(t, s, tg, u)
	(<-jobs)()
	c := findCall(t, r, "sendMessage")
	if c.Payload["reply_to_message_id"] != float64(99) || !strings.Contains(c.Payload["text"].(string), "AI Analysis") {
		t.Fatal("manual AI reply differs")
	}
	findCall(t, r, "sendChatAction")
	// Arbitrary text is not accepted in place of the original reply workflow.
	r.reset()
	handle(t, s, tg, botMessage("/ai some text"))
	if len(r.snapshot()) != 1 || !strings.Contains(findCall(t, r, "sendMessage").Payload["text"].(string), "reply") {
		t.Fatal("missing reply requirement")
	}
}

func TestPromptAndAnalysisRetainLegacyConstraints(t *testing.T) {
	prompt := buildSystemPrompt("zh", []string{"Focus on invoices"})
	for _, want := range []string{"HIGHEST PRIORITY", "Chinese", "[User Preferences]", "1. Focus on invoices", "source_language", "extracted_code", "50-100"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing constraint %q", want)
		}
	}
	a, err := parseAnalysis("```json\n{\"summary\":\"ok\"}\n```")
	if err != nil || a.Priority != 3 || a.Category != "notification" {
		t.Fatal("analysis defaults differ")
	}
	if _, err = parseAnalysis(`{"priority":9}`); err == nil {
		t.Fatal("invalid priority accepted")
	}
}

func TestCLIAccountBotAIAndSystemWizards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	var out bytes.Buffer
	script := strings.Join([]string{
		"2", "1", "1", "Work", "user@example.org", "app-secret", "INBOX,News", "https://mail.example.org", "n",
		"3", "1", "bot-secret", "42", "https://telegram.example.org",
		"4", "y", "ai-secret", "deepseek-chat", "", "3", "2",
		"5", "15", "4", "3", "y", "3", "proxy.example.org", "8080", "proxy-user", "proxy-secret",
		"7", "",
	}, "\n")
	if err := RunMenu(context.Background(), path, strings.NewReader(script), &out); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("wizard did not save valid config: %v\n%s", err, out.String())
	}
	if len(c.Accounts) != 1 || c.Accounts[0].Password != "app-secret" || len(c.Accounts[0].Folders) != 2 {
		t.Fatalf("account setup differs: %+v", c.Accounts)
	}
	if len(c.Notifiers) != 1 || c.Notifiers[0].Telegram.Token != "bot-secret" {
		t.Fatal("bot setup failed")
	}
	if !c.AI.Enabled || c.AI.Provider != "deepseek" || c.AI.Mode != "agent" || c.AI.Language != "zh" {
		t.Fatalf("AI setup failed: %+v", c.AI)
	}
	if c.PollInterval != 15 || c.MaxRetries != 4 || c.LogLevel != "WARNING" || c.Proxy == nil || c.Proxy.Scheme != "http" || c.Proxy.Password != "proxy-secret" {
		t.Fatal("system setup failed")
	}
	for _, secret := range []string{"app-secret", "bot-secret", "ai-secret", "proxy-secret"} {
		if strings.Contains(out.String(), secret) {
			t.Fatal("wizard printed a secret")
		}
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe config permissions: %v", info.Mode())
	}
	if err = RunMenu(context.Background(), path, strings.NewReader("2\n2\n1\n3\n2\n7\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
	c, err = LoadConfig(path)
	if err != nil || len(c.Accounts) != 0 || len(c.Notifiers) != 0 {
		t.Fatal("remove workflows failed")
	}
}

func TestCLIConnectionTestAndCancellation(t *testing.T) {
	s, _, r := parityService(t)
	if err := RunMenu(context.Background(), s.configPath, strings.NewReader("6\ny\n7\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
	findCall(t, r, "getMe")
	findCall(t, r, "sendMessage")
	before, _ := os.ReadFile(s.configPath)
	if err := RunMenu(context.Background(), s.configPath, strings.NewReader("5\n20\n:cancel\n7\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.configPath)
	if !bytes.Equal(before, after) {
		t.Fatal("cancelled wizard saved partial changes")
	}
}

func TestSettingsPreserveUnknownJSONAndOverrideStaleDatabase(t *testing.T) {
	s, tg, _ := parityService(t)
	var raw map[string]any
	b, _ := os.ReadFile(s.configPath)
	json.Unmarshal(b, &raw)
	raw["future_option"] = "keep"
	b, _ = json.Marshal(raw)
	os.WriteFile(s.configPath, b, 0600)
	if err := s.setLanguage(context.Background(), tg, "ja"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(s.configPath)
	json.Unmarshal(b, &raw)
	if raw["future_option"] != "keep" {
		t.Fatal("settings destroyed unrelated configuration")
	}
	if err := UpdateAISettings(s.configPath, map[string]string{"language": "en"}); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewService(c, s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	defer other.store.Close()
	defer other.network.Close()
	if err = other.acquireLock(); err != nil {
		t.Fatal(err)
	}
	defer other.lock.Close()
	if err = other.initializeSettings(); err != nil {
		t.Fatal(err)
	}
	if other.store.Setting("language", "") != "en" {
		t.Fatal("SQLite overrode edited JSON")
	}
}

func TestIMAPUnreadFilteringAndMarkSeenAfterQueueCommit(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(memory.New())
	srv.AllowInsecureAuth = true
	go srv.Serve(listener)
	defer srv.Close()
	c, err := client.Dial(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Terminate()
	c.Timeout = 2 * time.Second
	if err = c.Login("username", "password"); err != nil {
		t.Fatal(err)
	}
	s := &Service{config: DefaultConfig(), store: testStore(t), targets: []string{"target"}}
	a := Account{Name: "test", Email: "username", Host: "localhost", Port: 993}
	if err = s.pollFolder(c, a, "INBOX"); err != nil {
		t.Fatal(err)
	}
	body := "From: sender@example.org\r\nSubject: Mail\r\n\r\nbody"
	for _, flags := range [][]string{{imap.SeenFlag}, nil} {
		if err = c.Append("INBOX", flags, time.Now(), strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.pollFolder(c, a, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.store.Pending(); n != 1 {
		t.Fatalf("expected only unread mail, queued %d", n)
	}
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}
	uids, err := c.UidSearch(criteria)
	if err != nil || len(uids) != 0 {
		t.Fatalf("queued mail not marked seen: %v %v", uids, err)
	}
}

func TestHTMLSplittingPreservesCharactersAndBalancedTags(t *testing.T) {
	text := strings.Repeat("<&😀世界", 1200)
	parts := splitHTML("<b>Header</b>\n<code>" + escape(text) + "</code>")
	if len(parts) < 2 {
		t.Fatal("long body was not split")
	}
	var combined strings.Builder
	for _, p := range parts {
		if len(p) > 3900 {
			t.Fatalf("part exceeds safe length: %d", len(p))
		}
		if strings.Count(p, "<code>") != strings.Count(p, "</code>") {
			t.Fatal("code tag split incorrectly")
		}
		combined.WriteString(htmlText(p))
	}
	if strings.Count(combined.String(), "😀") != 1200 {
		t.Fatal("split lost Unicode content")
	}
}

func TestRotatingLogsAndSeparateConsoleLevel(t *testing.T) {
	previous := slog.Default()
	defer slog.SetDefault(previous)
	var console bytes.Buffer
	dir := t.TempDir()
	closer, err := ConfigureLogging("WARNING", dir, &console)
	if err != nil {
		t.Fatal(err)
	}
	slog.Debug("debug-file-only")
	slog.Info("info-file-only")
	slog.Warn("visible-warning")
	if strings.Contains(console.String(), "file-only") || !strings.Contains(console.String(), "visible-warning") {
		t.Fatal("console log filtering differs")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "mailbot.log"))
	if !bytes.Contains(b, []byte("debug-file-only")) {
		t.Fatal("file did not capture debug")
	}
	for range 8 {
		slog.Info("rotate", "body", strings.Repeat("x", 2<<20))
	}
	closer.Close()
	for _, suffix := range []string{"", ".1", ".2", ".3"} {
		if _, err = os.Stat(filepath.Join(dir, "mailbot.log"+suffix)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestChannelUsernameTargetsAuthorizeNumericCallbackChat(t *testing.T) {
	s, tg, r := parityService(t)
	tg.config.ChatID = "@NewsChannel"
	e := Email{ID: "channel-mail", Subject: "Channel mail", Body: "body"}
	if err := s.store.Commit("a", "INBOX", 1, 1, []Email{e}, s.targets); err != nil {
		t.Fatal(err)
	}
	u := botCallback("orig_" + e.ID)
	u.Callback.Message.Chat = chat{ID: "-1001234567890", Username: "newschannel"}
	handle(t, s, tg, u)
	c := findCall(t, r, "sendMessage")
	if c.Payload["chat_id"] != "-1001234567890" {
		t.Fatalf("reply routed incorrectly: %+v", c)
	}
	r.reset()
	u.Callback.Message.Chat.Username = "anotherchannel"
	handle(t, s, tg, u)
	if len(r.snapshot()) != 0 {
		t.Fatal("unconfigured channel could access mail")
	}
}

func TestExpiredCallbackClearsKeyboardAndExplainsExpiration(t *testing.T) {
	s, tg, r := parityService(t)
	handle(t, s, tg, botCallback("orig_missing"))
	c := findCall(t, r, "editMessageText")
	if !strings.Contains(c.Payload["text"].(string), "expired") {
		t.Fatal("missing expired-mail explanation")
	}
	markup := c.Payload["reply_markup"].(map[string]any)
	rows, ok := markup["inline_keyboard"].([]any)
	if !ok || len(rows) != 0 {
		t.Fatal("expired message keyboard must be an empty array")
	}
}
