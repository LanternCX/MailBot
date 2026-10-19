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
	registeredLanguage := ""
	var registerAttempt time.Time
	for ctx.Err() == nil {
		if registeredLanguage != s.uiLanguage() {
			registered = false
		}
		if !registered && time.Since(registerAttempt) > time.Minute {
			registerAttempt = time.Now()
			language := s.uiLanguage()
			commands := []map[string]string{{"command": "settings", "description": uiString(language, "settings")}, {"command": "help", "description": uiString(language, "help")}, {"command": "status", "description": uiString(language, "status")}}
			registered = t.call(ctx, "setMyCommands", map[string]any{"commands": commands}, nil) == nil
			if registered {
				registeredLanguage = language
			}
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
		return t.sendCard(ctx, Card{HTML: true, Text: "<b>MailBot</b>\n" + s.tr("intro")}, 0)
	case "/status":
		return t.send(ctx, s.Status(), nil)
	case "/settings":
		return t.sendCard(ctx, s.settingsCard("main"), 0)
	case "/mode":
		return t.send(ctx, s.tr("retired"), nil)
	case "/language":
		return s.setLanguage(ctx, t, arg)
	case "/rules":
		return t.send(ctx, s.tr("retired"), nil)
	case "/ai":
		if m.Reply == nil {
			return t.send(ctx, s.tr("reply_required"), nil)
		}
		if m.Reply.Text == "" {
			return t.send(ctx, s.tr("no_text"), nil)
		}
		return s.enqueueAI(ctx, t, Email{Subject: "(Manual Analysis)", Body: m.Reply.Text}, "manual", m.Reply.ID, jobs)
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
	case "settings_ui":
		ack("")
		return t.editCard(ctx, id, s.settingsCard("ui_language"))
	case "settings_lang":
		ack("")
		return t.editCard(ctx, id, s.settingsCard("language"))
	case "settings_mode":
		ack("")
		return t.editCard(ctx, id, s.settingsCard("main"))
	case "settings_back":
		ack("")
		return t.editCard(ctx, id, s.settingsCard("main"))
	case "settings_close", "rules_close":
		ack(s.tr("close"))
		return t.call(ctx, "deleteMessage", map[string]any{"chat_id": t.config.ChatID, "message_id": id}, nil)
	case "rules_add":
		ack(s.tr("retired"))
		return nil
	case "rules_delete":
		ack(s.tr("retired"))
		return nil
	}
	if strings.HasPrefix(data, "ui_lang_") {
		language := strings.TrimPrefix(data, "ui_lang_")
		if language != "zh" && language != "en" && language != "ja" {
			ack(s.tr("invalid"))
			return nil
		}
		if language == s.uiLanguage() {
			ack(s.tr("already"))
			return nil
		}
		if err := s.persistSetting("ui_language", language); err != nil {
			ack(s.tr("save_error"))
			return err
		}
		ack(s.tr("saved"))
		return t.editCard(ctx, id, s.settingsCard("main"))
	}
	if strings.HasPrefix(data, "lang_") || strings.HasPrefix(data, "language:") {
		language := strings.TrimPrefix(strings.TrimPrefix(data, "lang_"), "language:")
		if !validLanguage(language) {
			ack(s.tr("invalid"))
			return nil
		}
		if language == s.store.Setting("language", s.config.AI.Language) {
			ack(s.tr("already"))
			return nil
		}
		if err := s.persistSetting("language", language); err != nil {
			ack(s.tr("save_error"))
			return err
		}
		ack(s.tr("saved"))
		return t.editCard(ctx, id, s.settingsCard("main"))
	}
	if strings.HasPrefix(data, "mode_") || strings.HasPrefix(data, "mode:") {
		ack(s.tr("retired"))
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
		ack(s.tr("unknown"))
		return nil
	}
	ack(s.tr("loading"))
	e, err := s.store.EmailForTarget(emailID, t.key)
	if err != nil {
		return t.editCard(ctx, id, Card{Text: s.tr("expired")})
	}
	if action == "original" {
		return t.sendCard(ctx, originalCard(e, s.uiLanguage()), id)
	}
	return s.enqueueAI(ctx, t, e, action, id, jobs)
}

var languageLabels = map[string]string{"zh": "🇨🇳 中文", "en": "🇺🇸 English", "ja": "🇯🇵 日本語", "auto": "🌐 Auto-detect"}

func validLanguage(v string) bool { _, ok := languageLabels[v]; return ok }
func validMode(v string) bool     { return v == "raw" || v == "hybrid" || v == "agent" }
func (s *Service) settingsCard(page string) Card {
	language := s.readingLanguage()
	languageLabel := languageLabels[language]
	if languageLabel == "" {
		languageLabel = escape(language)
	}
	back := []button{{Text: s.tr("back"), Data: "settings_back"}}
	if page == "language" || page == "ui_language" {
		prefix, title := "lang_", s.tr("ai")
		if page == "ui_language" {
			language = s.uiLanguage()
			prefix = "ui_lang_"
			title = s.tr("ui")
		}
		rows := [][]button{}
		row := []button{}
		for _, code := range []string{"zh", "en", "ja"} {
			label := languageLabels[code]
			if code == language {
				label = "✅ " + label
			}
			row = append(row, button{Text: label, Data: prefix + code})
			if len(row) == 2 {
				rows = append(rows, row)
				row = nil
			}
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
		return Card{Text: "🌐 <b>" + title + "</b>\n\n" + s.tr("choose"), HTML: true, Buttons: append(rows, back)}
	}
	ai := s.tr("disabled")
	if s.config.AI.Enabled {
		ai = s.tr("enabled")
	}
	return Card{Text: "⚙️ <b>MailBot · " + s.tr("settings") + "</b>\n\n" + s.tr("ui") + ": <b>" + languageLabels[s.uiLanguage()] + "</b>\n" + s.tr("ai") + ": <b>" + languageLabel + "</b>\n" + ai, HTML: true, Buttons: [][]button{{{Text: s.tr("ui"), Data: "settings_ui"}, {Text: s.tr("ai"), Data: "settings_lang"}}, {{Text: s.tr("close"), Data: "settings_close"}}}}
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
	return t.send(ctx, s.tr("saved")+": "+v, nil)
}

func (s *Service) enqueueAI(ctx context.Context, t *Telegram, e Email, action string, replyID int, jobs chan<- func()) error {
	if !s.config.AI.Enabled {
		return t.send(ctx, s.tr("disabled"), nil)
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
			_ = t.sendCard(ctx, Card{Text: s.tr("ai_error")}, replyID)
			return
		}
		card := summaryCard(a, s.uiLanguage())
		switch action {
		case "manual":
			card = manualAnalysisCard(a, s.uiLanguage())
		case "translate":
			card = Card{Text: "🌐 <b>" + s.tr("translation") + "</b>\n" + s.tr("no_translation"), HTML: true}
			if a.Translation != nil && *a.Translation != "" {
				card.Text = "🌐 <b>" + s.tr("translation") + "</b>\n" + escape(*a.Translation)
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
		return t.send(ctx, s.tr("ai_busy"), nil)
	}
}

func (s *Service) Status() string {
	n, err := s.store.Pending()
	if err != nil {
		return s.tr("database_error")
	}
	text := fmt.Sprintf("MailBot\n%s: %t\n%s: %s\n%s: %s\n%s: %d", s.tr("summary"), s.config.AI.Enabled, s.tr("ui"), s.uiLanguage(), s.tr("ai"), s.readingLanguage(), s.tr("pending"), n)
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
			last := s.tr("never")
			if v := s.lastPoll[key]; !v.IsZero() {
				last = v.Format(time.RFC3339)
			}
			text += fmt.Sprintf("\n%s: %s · %s: %s", key, s.status[key], s.tr("last_check"), last)
		}
	}
	for i, t := range s.telegrams {
		last := s.tr("never")
		if v := s.lastSend[t.key]; !v.IsZero() {
			last = v.Format(time.RFC3339)
		}
		text += "\n" + s.tr("last_delivery", i+1, last)
	}
	return text
}
