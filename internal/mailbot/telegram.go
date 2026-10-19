package mailbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Telegram struct {
	config TelegramConfig
	client *http.Client
	key    string
}
type APIError struct {
	Code       int
	RetryAfter int
}

func (e *APIError) Error() string { return fmt.Sprintf("Telegram API status %d", e.Code) }

type button struct {
	Text     string    `json:"text"`
	Data     string    `json:"callback_data,omitempty"`
	URL      string    `json:"url,omitempty"`
	CopyText *copyText `json:"copy_text,omitempty"`
}

type copyText struct {
	Text string `json:"text"`
}

func (t *Telegram) call(ctx context.Context, method string, payload any, result any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(t.config.APIBase, "/")+"/bot"+t.config.Token+"/"+method, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("invalid Telegram endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("Telegram %s connection failed", method)
	}
	defer resp.Body.Close()
	var envelope struct {
		OK         bool            `json:"ok"`
		Result     json.RawMessage `json:"result"`
		Code       int             `json:"error_code"`
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("Telegram %s invalid response (HTTP %d)", method, resp.StatusCode)
	}
	if resp.StatusCode != 200 || !envelope.OK {
		code := envelope.Code
		if code == 0 {
			code = resp.StatusCode
		}
		return &APIError{code, envelope.Parameters.RetryAfter}
	}
	if result != nil {
		return json.Unmarshal(envelope.Result, result)
	}
	return nil
}

func (t *Telegram) send(ctx context.Context, text string, keyboard [][]button) error {
	payload := map[string]any{"chat_id": t.config.ChatID, "text": text, "disable_web_page_preview": false}
	if len(keyboard) > 0 {
		payload["reply_markup"] = map[string]any{"inline_keyboard": keyboard}
	}
	return t.call(ctx, "sendMessage", payload, nil)
}

func (t *Telegram) sendPart(ctx context.Context, text string, card Card, replyID int) error {
	payload := map[string]any{"chat_id": t.config.ChatID, "text": text, "disable_web_page_preview": card.DisablePreview}
	if card.HTML {
		payload["parse_mode"] = "HTML"
	}
	if replyID > 0 {
		payload["reply_to_message_id"] = replyID
	}
	if len(card.Buttons) > 0 {
		payload["reply_markup"] = map[string]any{"inline_keyboard": card.Buttons}
	}
	return t.call(ctx, "sendMessage", payload, nil)
}

func (t *Telegram) sendCard(ctx context.Context, card Card, replyID int) error {
	parts := card.Parts()
	for i, part := range parts {
		c := card
		if i < len(parts)-1 {
			c.Buttons = nil
		}
		if err := t.sendPart(ctx, part, c, replyID); err != nil {
			return err
		}
		if i < len(parts)-1 && !wait(ctx, time.Second) {
			return ctx.Err()
		}
	}
	return nil
}

func (t *Telegram) editCard(ctx context.Context, id int, card Card) error {
	parts := card.Parts()
	if len(parts) > 1 {
		return t.sendCard(ctx, card, id)
	}
	buttons := card.Buttons
	if buttons == nil {
		buttons = [][]button{}
	}
	p := map[string]any{"chat_id": t.config.ChatID, "message_id": id, "text": parts[0], "reply_markup": map[string]any{"inline_keyboard": buttons}}
	if card.HTML {
		p["parse_mode"] = "HTML"
	}
	return t.call(ctx, "editMessageText", p, nil)
}

func (t *Telegram) typing(ctx context.Context) {
	_ = t.call(ctx, "sendChatAction", map[string]any{"chat_id": t.config.ChatID, "action": "typing"}, nil)
}

// Telegram counts UTF-16 code units; this also keeps emoji within its limit.
func splitText(text string) []string {
	var parts []string
	var b strings.Builder
	units := 0
	for _, r := range text {
		n := 1
		if r > 0xffff {
			n = 2
		}
		if units+n > 3900 {
			parts = append(parts, b.String())
			b.Reset()
			units = 0
		}
		b.WriteRune(r)
		units += n
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	if len(parts) == 0 {
		parts = []string{"(Empty email)"}
	}
	return parts
}
func rawText(e Email) string {
	return previewText(e)
}
