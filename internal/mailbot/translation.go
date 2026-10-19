package mailbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

func readingLanguageLabel(language string) string {
	if label := map[string]string{"zh": "中文", "en": "English", "ja": "日本語"}[language]; label != "" {
		return label
	}
	return language
}

func (s *Service) serveTranslation(w http.ResponseWriter, r *http.Request, e Email, view readerView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	language := s.readingLanguage()
	var data []byte
	var a Analysis
	err := s.store.db.QueryRowContext(r.Context(), `SELECT data FROM reading_ai WHERE email=? AND language=? AND kind='translation'`, e.ID, language).Scan(&data)
	ready := err == nil && json.Unmarshal(data, &a) == nil && a.Translation != nil
	if r.Method == http.MethodPost && !ready {
		// A mail token grants reading access, not permission for another site to
		// silently trigger billable model calls in the reader's browser.
		origin := r.Header.Get("Origin")
		u, err := url.Parse(origin)
		if (origin != "" && (err != nil || u.Host != r.Host || (u.Scheme != "https" && u.Scheme != "http"))) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, s.tr("origin_error"), http.StatusForbidden)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
		defer cancel()
		a, err = s.readingAnalysis(ctx, e, language, "translation")
		if err != nil {
			view.Error = s.tr("translation_error")
			if len([]rune(e.Body)) > 60000 {
				view.Error = s.tr("too_long")
			}
		} else {
			http.Redirect(w, r, view.TranslateURL, http.StatusSeeOther)
			return
		}
	}
	if ready {
		view.Translation = *a.Translation
		view.Translated = true
	}
	_ = readerPage.Execute(w, view)
}
