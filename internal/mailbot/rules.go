package mailbot

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func escape(s string) string { return html.EscapeString(s) }

var ruleNumber = regexp.MustCompile(`^\p{Nd}+\.[\s\p{Zs}\x{0085}\x{001c}-\x{001f}]*`)
var anyNumber = regexp.MustCompile(`\d+`)

func loadRules(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("rules file must contain valid UTF-8")
	}
	var rules []string
	for _, line := range strings.FieldsFunc(string(data), func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\v' || r == '\f' || (r >= 0x1c && r <= 0x1e) || r == 0x85 || r == 0x2028 || r == 0x2029
	}) {
		line = ruleNumber.ReplaceAllString(strings.TrimFunc(line, func(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) }), "")
		if line != "" {
			rules = append(rules, line)
		}
	}
	return rules, nil
}
func saveRules(path string, rules []string) error {
	var out strings.Builder
	for i, rule := range rules {
		fmt.Fprintf(&out, "%d. %s\n", i+1, rule)
	}
	return atomicWrite(path, []byte(out.String()))
}
func (s *Service) ruleKey(t *Telegram) string { return t.config.Token + "\x00" + t.config.ChatID }
func (s *Service) beginRules(ctx context.Context, t *Telegram, action string) error {
	s.rulesMu.Lock()
	if s.rulesPending == nil {
		s.rulesPending = map[string]string{}
	}
	s.rulesPending[s.ruleKey(t)] = action
	s.rulesMu.Unlock()
	text := "📝 Send me the rule text you want to add.\n\nSend /rules to cancel."
	if action == "delete" {
		text = "🗑 Send me the rule number to delete (e.g. 2).\n\nSend /rules to cancel."
	}
	return t.send(ctx, text, nil)
}
func (s *Service) continueRules(ctx context.Context, t *Telegram, text string) error {
	s.rulesMu.Lock()
	action := s.rulesPending[s.ruleKey(t)]
	delete(s.rulesPending, s.ruleKey(t))
	s.rulesMu.Unlock()
	if action == "add" {
		return s.rulesCommand(ctx, t, "add "+text)
	}
	if action == "delete" {
		n := anyNumber.FindString(text)
		if n == "" {
			return t.send(ctx, "⚠️ Please send a rule number (e.g. 2).", nil)
		}
		return s.rulesCommand(ctx, t, "delete "+n)
	}
	return nil
}
func (s *Service) rulesCommand(ctx context.Context, t *Telegram, arg string) error {
	if arg == "add" || arg == "delete" {
		return s.beginRules(ctx, t, arg)
	}
	s.rulesMu.Lock()
	defer s.rulesMu.Unlock()
	delete(s.rulesPending, s.ruleKey(t))
	path := filepath.Join(s.configDir, "rules.md")
	rules, err := loadRules(path)
	if err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(arg, "add "):
		rule := strings.TrimSpace(strings.TrimPrefix(arg, "add "))
		if rule == "" {
			return t.send(ctx, "Rule text required.", nil)
		}
		rules = append(rules, rule)
	case strings.HasPrefix(arg, "delete "):
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "delete ")))
		if err != nil || n < 1 || n > len(rules) {
			return t.send(ctx, "⚠️ Rule number not found.", nil)
		}
		rules = append(rules[:n-1], rules[n:]...)
	case arg == "clear":
		rules = nil
	default:
		text := "📋 <b>Persona Rules</b>\n\nNo rules yet.\n\nUse /rules add to add a rule."
		if len(rules) > 0 {
			var out strings.Builder
			fmt.Fprintf(&out, "📋 <b>Persona Rules</b> (%d)\n\n", len(rules))
			for i, rule := range rules {
				fmt.Fprintf(&out, "%d. %s\n", i+1, escape(rule))
			}
			out.WriteString("\nUse /rules add or /rules delete to manage.")
			text = out.String()
		}
		return t.sendCard(ctx, Card{Text: text, HTML: true, Buttons: [][]button{{{Text: "➕ Add", Data: "rules_add"}, {Text: "🗑 Delete", Data: "rules_delete"}}, {{Text: "❌ Close", Data: "rules_close"}}}}, 0)
	}
	if err = saveRules(path, rules); err != nil {
		return err
	}
	return t.send(ctx, fmt.Sprintf("✅ Rules saved (total: %d).", len(rules)), nil)
}
