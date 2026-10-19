package mailbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (s *Service) readingLanguage() string {
	language := s.store.Setting("language", s.config.AI.Language)
	if language == "" || language == "auto" {
		return "zh"
	}
	return language
}

// Persist successful results by mail and language. Failed calls never become a
// cached translation; a later explicit retry can recover after an outage.
func (s *Service) readingAnalysis(ctx context.Context, e Email, language, kind string) (Analysis, error) {
	read := func() (Analysis, bool) {
		var data []byte
		var a Analysis
		err := s.store.db.QueryRowContext(ctx, `SELECT data FROM reading_ai WHERE email=? AND language=? AND kind=?`, e.ID, language, kind).Scan(&data)
		if err != nil || json.Unmarshal(data, &a) != nil {
			return Analysis{}, false
		}
		return a, true
	}
	if a, ok := read(); ok {
		return a, nil
	}
	if !s.config.AI.Enabled || s.network == nil {
		return Analysis{}, fmt.Errorf("AI is unavailable")
	}
	s.readingOnce.Do(func() { s.readingSlots = make(chan struct{}, 1) })
	// Serialize generation, including the cache recheck, so parallel page loads
	// and different Telegram targets cannot charge for the same result twice.
	select {
	case s.readingSlots <- struct{}{}:
		defer func() { <-s.readingSlots }()
	case <-ctx.Done():
		return Analysis{}, ctx.Err()
	}
	if a, ok := read(); ok {
		return a, nil
	}
	var a Analysis
	var err error
	if kind == "translation" {
		runes := []rune(e.Body)
		if len(runes) == 0 || len(runes) > 60000 {
			return Analysis{}, fmt.Errorf("email is empty or too long to translate")
		}
		var parts []string
		for len(runes) > 0 {
			n := min(len(runes), 3000)
			// Prefer paragraph boundaries without ever dropping source text.
			if n < len(runes) {
				for i := n - 1; i >= n/2; i-- {
					if runes[i] == '\n' {
						n = i + 1
						break
					}
				}
			}
			chunk := e
			chunk.Body = string(runes[:n])
			a, err = s.analyzeLanguage(ctx, chunk, language, true)
			if err != nil {
				return Analysis{}, err
			}
			parts = append(parts, *a.Translation)
			runes = runes[n:]
		}
		text := strings.Join(parts, "\n\n")
		a.Translation = &text
	} else {
		a, err = s.analyzeLanguage(ctx, e, language, false)
		if err != nil {
			return Analysis{}, err
		}
	}
	data, err := json.Marshal(a)
	if err != nil {
		return Analysis{}, err
	}
	_, err = s.store.db.ExecContext(ctx, `INSERT OR REPLACE INTO reading_ai(email,language,kind,data) SELECT ?,?,?,? WHERE EXISTS(SELECT 1 FROM emails WHERE id=?)`, e.ID, language, kind, data, e.ID)
	return a, err
}

func (s *Service) notificationCard(ctx context.Context, e Email, link string) Card {
	card := readingPreviewCard(e, link, e.Body, 180, s.uiLanguage())
	if link == "" {
		card = originalCard(e, s.uiLanguage())
	}
	if !s.config.AI.Enabled || verificationMail(e) {
		return card
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	a, err := s.readingAnalysis(ctx, e, s.readingLanguage(), "summary")
	if err != nil {
		return card
	}
	if link == "" {
		return Card{Text: "📬 <b>" + escape(e.Subject) + "</b>\n" + escape(e.Sender) + "\n\n" + escape(truncate(a.Summary, 600)), HTML: true, DisablePreview: true}
	}
	// Use the same compact card and original-mail URL; only replace the preview.
	return readingPreviewCard(e, link, a.Summary, 600, s.uiLanguage())
}
