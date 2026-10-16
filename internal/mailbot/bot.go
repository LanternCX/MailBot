package mailbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

type chat struct {
	ID       json.Number `json:"id"`
	Username string      `json:"username"`
}
type message struct {
	ID    int      `json:"message_id"`
	Text  string   `json:"text"`
	Chat  chat     `json:"chat"`
	Reply *message `json:"reply_to_message"`
}
type callback struct {
	ID      string   `json:"id"`
	Data    string   `json:"data"`
	Message *message `json:"message"`
}
type update struct {
	ID       int64     `json:"update_id"`
	Message  *message  `json:"message"`
	Callback *callback `json:"callback_query"`
}

func (s *Service) botLoop(ctx context.Context, t *Telegram) {
	ctx, cancel := context.WithCancel(ctx)
	key := "offset:" + t.key
	offset, _ := strconv.ParseInt(s.store.Setting(key, "0"), 10, 64)
	jobs := make(chan func(), 8)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job := <-jobs:
					job()
				}
			}
		}()
	}
	defer func() { cancel(); wg.Wait() }()
	failures := 0
	registered := false
	var registerAttempt time.Time
	for ctx.Err() == nil {
		if !registered && time.Since(registerAttempt) > time.Minute {
			registerAttempt = time.Now()
			commands := []map[string]string{{"command": "settings", "description": "Open settings dashboard"}, {"command": "rules", "description": "View / manage persona rules"}, {"command": "ai", "description": "Reply to a message to analyze with AI"}, {"command": "help", "description": "Show help"}, {"command": "status", "description": "Show service health"}}
			registered = t.call(ctx, "setMyCommands", map[string]any{"commands": commands}, nil) == nil
		}
		pollTimeout := t.config.Timeout - 5
		if pollTimeout < 1 {
			pollTimeout = 1
		}
		if pollTimeout > 25 {
			pollTimeout = 25
		}
		var updates []update
		err := t.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": pollTimeout, "allowed_updates": []string{"message", "callback_query"}}, &updates)
		if err != nil {
			failures++
			slog.Warn("Telegram polling failed; reconnecting", "error", err)
			if !wait(ctx, backoff(failures)) {
				return
			}
			continue
		}
		failures = 0
		for _, u := range updates {
			if ctx.Err() != nil {
				return
			}
			if err = s.handleUpdate(ctx, t, u, jobs); err != nil {
				slog.Warn("Telegram command failed", "error", err)
			}
			offset = u.ID + 1
			if err = s.store.Set(key, strconv.FormatInt(offset, 10)); err != nil {
				slog.Error("Telegram offset save failed", "error", err)
				return
			}
		}
		if len(updates) == 0 && !wait(ctx, 100*time.Millisecond) {
			return
		}
	}
}

func matchesChat(configured string, c chat) bool {
	return configured == c.ID.String() || (c.Username != "" && strings.EqualFold(configured, "@"+c.Username))
}
func (s *Service) allowedChat(t *Telegram, c chat) bool {
	for _, target := range s.telegrams {
		if target.config.Token == t.config.Token && matchesChat(target.config.ChatID, c) {
			return true
		}
	}
	return false
}

func (s *Service) handleUpdate(ctx context.Context, t *Telegram, u update, jobs chan<- func()) error {
	m := u.Message
	if u.Callback != nil {
		m = u.Callback.Message
	}
	if m == nil || !s.allowedChat(t, m.Chat) {
		return nil
	}
	target := *t
	target.config.ChatID = m.Chat.ID.String()
	t = &target
	for _, x := range s.telegrams {
		if x.config.Token == t.config.Token && matchesChat(x.config.ChatID, m.Chat) {
			t.key = x.key
		}
	}
	if u.Callback != nil {
		return s.handleCallback(ctx, t, u.Callback, jobs)
	}
	fields := strings.Fields(m.Text)
	if len(fields) == 0 {
		return nil
	}
	command := strings.SplitN(fields[0], "@", 2)[0]
	arg := strings.TrimSpace(strings.TrimPrefix(m.Text, fields[0]))
	switch command {
	case "/start", "/help":
		return t.sendCard(ctx, Card{HTML: true, Text: "🤖 <b>MailBot Help</b>\n/settings — Language &amp; operation mode\n/rules — View / manage persona rules\n/ai — Reply to a message to analyze with AI\n/status — Service health\n\nRaw: forward only\nHybrid: preview with on-demand AI\nAgent: automatic AI summary\n\nUse the CLI or config.json to change AI on/off."}, 0)
	case "/status":
		return t.send(ctx, s.Status(), nil)
	case "/settings":
		return t.sendCard(ctx, s.settingsCard("main"), 0)
	case "/mode":
		if arg == "" {
			return t.sendCard(ctx, s.settingsCard("main"), 0)
		}
		return s.setMode(ctx, t, arg)
	case "/language":
		return s.setLanguage(ctx, t, arg)
	case "/rules":
		return s.rulesCommand(ctx, t, arg)
	case "/ai":
		if m.Reply == nil {
			return t.send(ctx, "⚠️ Please reply to a message before using /ai to analyze it.", nil)
		}
		if m.Reply.Text == "" {
			return t.send(ctx, "⚠️ The replied message has no text to analyze.", nil)
		}
		return s.enqueueAI(ctx, t, Email{Subject: "(Manual Analysis)", Body: m.Reply.Text}, "manual", m.Reply.ID, jobs)
	}
	if !strings.HasPrefix(command, "/") {
		return s.continueRules(ctx, t, m.Text)
	}
	return nil
}

func (s *Service) handleCallback(ctx context.Context, t *Telegram, cq *callback, jobs chan<- func()) error {
	data := cq.Data
	id := cq.Message.ID
	ack := func(text string) {
		_ = t.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": cq.ID, "text": text}, nil)
	}
	switch data {
	case "settings_lang":
		ack("")
		return t.editCard(ctx, id, s.settingsCard("language"))
	case "settings_mode":
		ack("")
		return t.editCard(ctx, id, s.settingsCard("mode"))
	case "settings_back":
		ack("")
		return t.editCard(ctx, id, s.settingsCard("main"))
	case "settings_close", "rules_close":
		ack("Closed")
		return t.call(ctx, "deleteMessage", map[string]any{"chat_id": t.config.ChatID, "message_id": id}, nil)
	case "rules_add":
		ack("Send me the rule text")
		return s.beginRules(ctx, t, "add")
	case "rules_delete":
		ack("Send a rule number to delete")
		return s.beginRules(ctx, t, "delete")
	}
	if strings.HasPrefix(data, "lang_") || strings.HasPrefix(data, "language:") {
		language := strings.TrimPrefix(strings.TrimPrefix(data, "lang_"), "language:")
		if !validLanguage(language) {
			ack("Invalid language")
			return nil
		}
		if language == s.store.Setting("language", s.config.AI.Language) {
			ack("Already set to " + languageLabels[language])
			return nil
		}
		if err := s.persistSetting("language", language); err != nil {
			ack("Unable to save language; please retry")
			return err
		}
		ack("✅ Language set to " + languageLabels[language])
		return t.editCard(ctx, id, s.settingsCard("main"))
	}
	if strings.HasPrefix(data, "mode_") || strings.HasPrefix(data, "mode:") {
		mode := strings.TrimPrefix(strings.TrimPrefix(data, "mode_"), "mode:")
		if !validMode(mode) {
			ack("Invalid mode")
			return nil
		}
		if mode == s.mode() {
			ack("Already in " + mode + " mode")
			return nil
		}
		if err := s.persistSetting("mode", mode); err != nil {
			ack("Unable to save mode; please retry")
			return err
		}
		ack("✅ Switched to " + mode)
		return t.editCard(ctx, id, s.settingsCard("main"))
	}
	action, emailID := "", ""
	for _, prefix := range []struct{ prefix, action string }{{"summ_", "summary"}, {"orig_", "original"}, {"trans_", "translate"}, {"summary:", "summary"}, {"original:", "original"}, {"translate:", "translate"}} {
		if strings.HasPrefix(data, prefix.prefix) {
			action = prefix.action
			emailID = strings.TrimPrefix(data, prefix.prefix)
			break
		}
	}
	if action == "" {
		ack("Unknown action")
		return nil
	}
	ack("Loading " + action + "…")
	e, err := s.store.EmailForTarget(emailID, t.key)
	if err != nil {
		return t.editCard(ctx, id, Card{Text: "⚠️ Email cache expired; cannot load this email."})
	}
	if action == "original" {
		return t.sendCard(ctx, originalCard(e), id)
	}
	return s.enqueueAI(ctx, t, e, action, id, jobs)
}

var languageLabels = map[string]string{"zh": "🇨🇳 中文", "en": "🇺🇸 English", "ja": "🇯🇵 日本語", "auto": "🌐 Auto-detect"}

func validLanguage(v string) bool { _, ok := languageLabels[v]; return ok }
func validMode(v string) bool     { return v == "raw" || v == "hybrid" || v == "agent" }
func (s *Service) settingsCard(page string) Card {
	mode := s.mode()
	language := s.store.Setting("language", s.config.AI.Language)
	languageLabel := languageLabels[language]
	if languageLabel == "" {
		languageLabel = escape(language)
	}
	back := []button{{Text: "🔙 Back", Data: "settings_back"}}
	if page == "language" {
		rows := [][]button{}
		row := []button{}
		for _, code := range []string{"zh", "en", "ja", "auto"} {
			label := languageLabels[code]
			if code == language {
				label = "✅ " + label
			}
			row = append(row, button{Text: label, Data: "lang_" + code})
			if len(row) == 2 {
				rows = append(rows, row)
				row = nil
			}
		}
		return Card{Text: "🌐 <b>Language Settings</b>\n\nCurrent: ✅ <b>" + languageLabel + "</b>\n\nSelect output language:", HTML: true, Buttons: append(rows, back)}
	}
	if page == "mode" {
		rows := [][]button{}
		for _, v := range []string{"raw", "hybrid", "agent"} {
			label := strings.ToUpper(v[:1]) + v[1:]
			if mode == v {
				label = "✅ " + label
			}
			rows = append(rows, []button{{Text: label, Data: "mode_" + v}})
		}
		return Card{Text: "⚙️ <b>Operation Mode</b>\n\nCurrent: ✅ <b>" + mode + "</b>\n\nSelect mode:", HTML: true, Buttons: append(rows, back)}
	}
	return Card{Text: "⚙️ <b>MailBot Settings</b>\n\n🌐 Language: <b>" + languageLabel + "</b>\n📋 Mode: <b>" + mode + "</b>\nUse the CLI or config.json to change AI on/off.", HTML: true, Buttons: [][]button{{{Text: "🌐 Language >", Data: "settings_lang"}}, {{Text: "⚙️ Mode: " + mode + " >", Data: "settings_mode"}}, {{Text: "❌ Close", Data: "settings_close"}}}}
}

func (s *Service) persistSetting(key, value string) error {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	field := key
	if key == "mode" {
		field = "default_mode"
	}
	if s.configPath == "" {
		return fmt.Errorf("configuration path unavailable")
	}
	if err := UpdateAISettings(s.configPath, map[string]string{field: value}); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return s.store.Set(key, value)
}
func (s *Service) setMode(ctx context.Context, t *Telegram, v string) error {
	if !validMode(v) {
		return t.send(ctx, "Use /mode raw|hybrid|agent", nil)
	}
	if err := s.persistSetting("mode", v); err != nil {
		return err
	}
	return t.send(ctx, "Mode: "+v, nil)
}
func (s *Service) setLanguage(ctx context.Context, t *Telegram, v string) error {
	if !validLanguage(v) {
		return t.send(ctx, "Use /language auto|zh|en|ja", nil)
	}
	if err := s.persistSetting("language", v); err != nil {
		return err
	}
	return t.send(ctx, "Language: "+v, nil)
}

func (s *Service) enqueueAI(ctx context.Context, t *Telegram, e Email, action string, replyID int, jobs chan<- func()) error {
	if !s.config.AI.Enabled {
		return t.send(ctx, "AI is disabled.", nil)
	}
	job := func() {
		select {
		case s.aiSlots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-s.aiSlots }()
		t.typing(ctx)
		a, err := s.analyze(ctx, e)
		if err != nil {
			_ = t.sendCard(ctx, Card{Text: "AI analysis failed. Try again later."}, replyID)
			return
		}
		card := summaryCard(a)
		switch action {
		case "manual":
			card = manualAnalysisCard(a)
		case "translate":
			card = Card{Text: "🌐 <b>Translation</b>\n⚠️ No translation available.", HTML: true}
			if a.Translation != nil && *a.Translation != "" {
				card.Text = "🌐 <b>Translation</b>\n" + escape(*a.Translation)
			}
		}
		if action != "manual" {
			card.Text += webAnchor(e.WebLink)
		}
		if err = t.sendCard(ctx, card, replyID); err != nil {
			slog.Warn("AI reply failed", "error", err)
		}
	}
	select {
	case jobs <- job:
		return nil
	default:
		return t.send(ctx, "AI is busy. Try again later.", nil)
	}
}

func (s *Service) Status() string {
	n, err := s.store.Pending()
	if err != nil {
		return "State database unavailable."
	}
	text := fmt.Sprintf("MailBot\nMode: %s\nPending deliveries: %d", s.mode(), n)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.config.Accounts {
		if !a.Enabled {
			continue
		}
		for _, folder := range a.Folders {
			mailbox := a
			mailbox.Folders = []string{folder}
			key := mailboxStatusKey(mailbox)
			last := "never"
			if v := s.lastPoll[key]; !v.IsZero() {
				last = v.Format(time.RFC3339)
			}
			text += fmt.Sprintf("\n%s: %s · last successful check: %s", key, s.status[key], last)
		}
	}
	for i, t := range s.telegrams {
		last := "never"
		if v := s.lastSend[t.key]; !v.IsZero() {
			last = v.Format(time.RFC3339)
		}
		text += fmt.Sprintf("\nTarget %d last delivery: %s", i+1, last)
	}
	return text
}
