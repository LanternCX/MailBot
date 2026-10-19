package mailbot

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInterfaceAndAILanguagesAreIndependentAndPersistent(t *testing.T) {
	s, tg, r := parityService(t)
	if err := s.persistSetting("language", "en"); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"en", "ja", "zh"} {
		r.reset()
		handle(t, s, tg, botCallback("ui_lang_"+language))
		cfg, err := LoadConfig(s.configPath)
		if err != nil || cfg.UILanguage != language || cfg.AI.Language != "en" || s.readingLanguage() != "en" {
			t.Fatalf("UI change affected AI or failed persistence: %+v %v", cfg, err)
		}
		card := findCall(t, r, "editMessageText")
		if !strings.Contains(card.Payload["text"].(string), uiString(language, "ui")) {
			t.Fatal("settings did not switch language")
		}
		// initializeSettings reads the startup config; simulate a restart with saved JSON.
		s.config = cfg
		if err := s.initializeSettings(); err != nil {
			t.Fatal(err)
		}
		if s.uiLanguage() != language || s.readingLanguage() != "en" {
			t.Fatal("language changed after restart")
		}
	}
	if err := s.persistSetting("ui_language", "en"); err != nil {
		t.Fatal(err)
	}
	if err := s.persistSetting("language", "ja"); err != nil {
		t.Fatal(err)
	}
	if s.uiLanguage() != "en" {
		t.Fatal("AI language changed UI language")
	}
	card := s.settingsCard("language")
	var hasJapanese bool
	for _, row := range card.Buttons {
		for _, b := range row {
			hasJapanese = hasJapanese || b.Data == "lang_ja"
		}
	}
	if !hasJapanese {
		t.Fatal("Japanese option missing")
	}
}

func TestReaderAndNotificationsUseInterfaceLanguage(t *testing.T) {
	s := readingAIService(t, nil)
	s.config.AI.Enabled = true
	s.config.AI.Language = "en"
	e := Email{ID: "localized", Subject: "Verification code", Body: "Your verification code is 482731", HTMLBody: "<p>Hello</p>"}
	if err := s.store.Commit("a", "INBOX", 1, 1, []Email{e}, []string{"t"}); err != nil {
		t.Fatal(err)
	}
	token, _ := s.store.readingToken(e.ID)
	for _, language := range []string{"zh", "en", "ja"} {
		if err := s.store.Set("ui_language", language); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		s.readerHandler().ServeHTTP(w, httptest.NewRequest("GET", "/read/"+token, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `lang="`+language+`"`) || !strings.Contains(w.Body.String(), uiString(language, "private")) {
			t.Fatal("reader language mismatch")
		}
		if !strings.Contains(w.Body.String(), uiString(language, "translate", "English")) {
			t.Fatal("translation button confused interface and AI languages")
		}
		w = httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/read/"+token+"/translation", nil)
		request.Header.Set("Origin", "https://other.example")
		s.readerHandler().ServeHTTP(w, request)
		if w.Code != 403 || !strings.Contains(w.Body.String(), uiString(language, "origin_error")) {
			t.Fatal("translation error language mismatch")
		}
		card := s.notificationCard(context.Background(), e, "https://mail.example/read/"+token)
		if card.Buttons[0][0].Text != uiString(language, "body") || card.Buttons[0][1].Text != uiString(language, "copy") || card.Buttons[0][1].CopyText.Text != "482731" {
			t.Fatal("notification language or code mismatch")
		}
		if s.readingLanguage() != "en" {
			t.Fatal("UI language affected AI")
		}
	}
}
