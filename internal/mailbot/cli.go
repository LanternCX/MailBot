package mailbot

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

var errCancel = errors.New("cancelled")

type terminal struct {
	scanner *bufio.Scanner
	out     io.Writer
	input   io.Reader
}

func (t *terminal) ask(label, def string, secret bool) (string, error) {
	if secret && def != "" {
		fmt.Fprintf(t.out, "%s [saved; Enter to keep]: ", label)
	} else if def != "" {
		fmt.Fprintf(t.out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(t.out, "%s: ", label)
	}
	var value string
	if f, ok := t.input.(*os.File); secret && ok && term.IsTerminal(int(f.Fd())) {
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(t.out)
		if err != nil {
			return "", err
		}
		value = string(b)
	} else {
		if !t.scanner.Scan() {
			if err := t.scanner.Err(); err != nil {
				return "", err
			}
			return "", io.EOF
		}
		value = t.scanner.Text()
	}
	if value == ":cancel" {
		return "", errCancel
	}
	if value == "" {
		return def, nil
	}
	return strings.TrimSpace(value), nil
}
func (t *terminal) choice(label string, options []string, def int) (int, error) {
	fmt.Fprintln(t.out, label)
	for i, opt := range options {
		fmt.Fprintf(t.out, "  %d. %s\n", i+1, opt)
	}
	for {
		raw, err := t.ask("Choose", strconv.Itoa(def+1), false)
		if err != nil {
			return 0, err
		}
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		for i, opt := range options {
			if strings.EqualFold(raw, opt) {
				return i, nil
			}
		}
		fmt.Fprintln(t.out, "Choose a listed option.")
	}
}
func (t *terminal) integer(label string, def, min, max int) (int, error) {
	for {
		v, err := t.ask(label, strconv.Itoa(def), false)
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(v)
		if err == nil && n >= min && n <= max {
			return n, nil
		}
		fmt.Fprintf(t.out, "Enter an integer from %d to %d.\n", min, max)
	}
}
func (t *terminal) yes(label string, def bool) (bool, error) {
	d := "n"
	if def {
		d = "y"
	}
	for {
		v, err := t.ask(label+" (y/n)", d, false)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(v) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintln(t.out, "Enter y or n.")
	}
}
func (t *terminal) required(label, def string, secret bool) (string, error) {
	for {
		v, err := t.ask(label, def, secret)
		if err != nil {
			return "", err
		}
		if v != "" {
			return v, nil
		}
		fmt.Fprintln(t.out, "A value is required.")
	}
}

func RunMenu(ctx context.Context, path string, input io.Reader, out io.Writer) error {
	if f, ok := input.(*os.File); ok {
		stop := context.AfterFunc(ctx, func() { f.Close() })
		defer stop()
	}
	t := &terminal{scanner: bufio.NewScanner(input), input: input, out: out}
	t.scanner.Buffer(make([]byte, 4096), 1<<20)
	fmt.Fprintln(out, "MailBot — IMAP → Telegram\nEnter :cancel to leave a dialog without saving.")
	for ctx.Err() == nil {
		c := DefaultConfig()
		if loaded, err := LoadConfig(path); err == nil {
			c = loaded
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(out, "Configuration could not be loaded: %v\n", err)
		}
		choice, err := t.choice("Main Menu", []string{"Start Service", "Config Wizard", "Bot Settings", "AI Settings", "System Settings", "Test Connection", "Exit"}, 0)
		if err == io.EOF || choice == 6 && err == nil {
			fmt.Fprintln(out, "Bye.")
			return nil
		}
		if err != nil {
			return err
		}
		switch choice {
		case 0:
			if len(c.Accounts) == 0 || len(c.Notifiers) == 0 {
				fmt.Fprintln(out, "Configure accounts and a Telegram bot first.")
				continue
			}
			var service *Service
			service, err = NewService(c, path)
			if err != nil {
				break
			}
			var logs io.Closer
			logs, err = ConfigureLogging(c.LogLevel, filepath.Join(filepath.Dir(path), "logs"), out)
			if err != nil {
				service.store.Close()
				service.network.Close()
				break
			}
			child, stop := signal.NotifyContext(ctx, os.Interrupt)
			fmt.Fprintln(out, "Service started — Ctrl+C to stop")
			err = service.Run(child)
			stop()
			logs.Close()
			fmt.Fprintln(out, "Service stopped.")
		case 1:
			err = t.accounts(ctx, &c)
		case 2:
			err = t.bot(&c)
		case 3:
			err = t.ai(&c)
		case 4:
			err = t.system(&c)
		case 5:
			err = t.testConnection(ctx, c)
		}
		if err == io.EOF {
			return nil
		}
		if errors.Is(err, errCancel) {
			fmt.Fprintln(out, "No changes applied.")
			continue
		}
		if err != nil {
			fmt.Fprintf(out, "Error: %v\n", err)
			continue
		}
		if choice >= 1 && choice <= 4 {
			if err = SaveConfig(path, c); err != nil {
				fmt.Fprintf(out, "Save failed: %v\n", err)
			} else {
				fmt.Fprintln(out, "Configuration saved.")
			}
		}
	}
	return nil
}

func (t *terminal) accounts(ctx context.Context, c *Config) error {
	for i, a := range c.Accounts {
		fmt.Fprintf(t.out, "%d. %s | %s | %s:%d | enabled=%v | SSL=%v\n", i+1, a.Name, a.Email, a.Host, a.Port, a.Enabled, a.SSL)
	}
	choice, err := t.choice("Account action", []string{"Add Account", "Remove Account", "Back"}, 0)
	if err != nil {
		return err
	}
	if choice == 2 {
		return errCancel
	}
	if choice == 1 {
		if len(c.Accounts) == 0 {
			fmt.Fprintln(t.out, "No accounts to remove.")
			return errCancel
		}
		options := []string{}
		for _, a := range c.Accounts {
			options = append(options, a.Name+" ("+a.Email+")")
		}
		options = append(options, "Cancel")
		n, err := t.choice("Remove which account?", options, len(options)-1)
		if err != nil {
			return err
		}
		if n == len(c.Accounts) {
			return errCancel
		}
		c.Accounts = append(c.Accounts[:n], c.Accounts[n+1:]...)
		return nil
	}
	providers := []string{"Gmail", "Outlook", "QQ Mail", "163 Mail", "Custom"}
	hosts := []string{"imap.gmail.com", "outlook.office365.com", "imap.qq.com", "imap.163.com", ""}
	n, err := t.choice("Select provider", providers, 0)
	if err != nil {
		return err
	}
	a := Account{Host: hosts[n], Port: 993, SSL: true, Enabled: true, Folders: []string{"INBOX"}}
	if a.Name, err = t.required("Display name", providers[n], false); err != nil {
		return err
	}
	for {
		a.Email, err = t.required("Email address", "", false)
		if err != nil {
			return err
		}
		if strings.Contains(a.Email, "@") {
			break
		}
		fmt.Fprintln(t.out, "Invalid email address.")
	}
	if a.Password, err = t.required("App password", "", true); err != nil {
		return err
	}
	if n == 4 {
		if a.Host, err = t.required("IMAP host", "", false); err != nil {
			return err
		}
		if a.Port, err = t.integer("IMAP port", 993, 1, 65535); err != nil {
			return err
		}
		if a.SSL, err = t.yes("Use SSL?", true); err != nil {
			return err
		}
	}
	folders, err := t.ask("Folders (comma separated)", "INBOX", false)
	if err != nil {
		return err
	}
	a.Folders = nil
	for _, folder := range strings.Split(folders, ",") {
		if v := strings.TrimSpace(folder); v != "" {
			a.Folders = append(a.Folders, v)
		}
	}
	if len(a.Folders) == 0 {
		return fmt.Errorf("at least one folder required")
	}
	if a.WebURL, err = t.ask("Webmail URL prefix (optional)", "", false); err != nil {
		return err
	}
	verify, err := t.yes("Verify IMAP connection now?", true)
	if err != nil {
		return err
	}
	if verify {
		if err = CheckIMAP(ctx, *c, a); err != nil {
			fmt.Fprintf(t.out, "IMAP check failed: %v\n", err)
		} else {
			fmt.Fprintln(t.out, "Success: IMAP login OK.")
		}
	}
	c.Accounts = append(c.Accounts, a)
	return nil
}

func (t *terminal) bot(c *Config) error {
	tg := TelegramConfig{APIBase: "https://api.telegram.org", Timeout: 30}
	for _, n := range c.Notifiers {
		if n.Telegram != nil {
			tg = *n.Telegram
			fmt.Fprintf(t.out, "Telegram chat=%s | token=[hidden]\n", tg.ChatID)
			break
		}
	}
	n, err := t.choice("Bot action", []string{"Set Telegram Bot", "Remove Bot", "Back"}, 0)
	if err != nil {
		return err
	}
	if n == 2 {
		return errCancel
	}
	if n == 1 {
		c.Notifiers = []Notifier{}
		return nil
	}
	if tg.Token, err = t.required("Bot token", tg.Token, true); err != nil {
		return err
	}
	if tg.ChatID, err = t.required("Chat ID", tg.ChatID, false); err != nil {
		return err
	}
	if tg.APIBase, err = t.required("API base URL", tg.APIBase, false); err != nil {
		return err
	}
	c.Notifiers = []Notifier{{Type: "telegram", Enabled: true, Telegram: &tg}}
	return nil
}

func (t *terminal) ai(c *Config) error {
	enabled, err := t.yes("Enable AI analysis?", c.AI.Enabled)
	if err != nil {
		return err
	}
	if !enabled {
		c.AI = DefaultConfig().AI
		return nil
	}
	a := c.AI
	a.Enabled = true
	a.Provider = "deepseek"
	if c.AI.Provider != "deepseek" {
		a.Model = "deepseek-chat"
		a.BaseURL = ""
		a.APIKey = ""
	}
	fmt.Fprintln(t.out, "AI provider: DeepSeek (OpenAI-compatible API)")
	if a.APIKey, err = t.ask("DeepSeek API Key", a.APIKey, true); err != nil {
		return err
	}
	if a.Model, err = t.required("Model name", a.Model, false); err != nil {
		return err
	}
	if a.BaseURL, err = t.ask("API Base URL (optional)", a.BaseURL, false); err != nil {
		return err
	}
	modes := []string{"raw", "hybrid", "agent"}
	d := 1
	for i, v := range modes {
		if v == a.Mode {
			d = i
		}
	}
	n, err := t.choice("Default operation mode", modes, d)
	if err != nil {
		return err
	}
	a.Mode = modes[n]
	languages := []string{"auto", "zh", "en", "ja"}
	d = 0
	for i, v := range languages {
		if v == a.Language {
			d = i
		}
	}
	n, err = t.choice("AI output language", languages, d)
	if err != nil {
		return err
	}
	a.Language = languages[n]
	c.AI = a
	return nil
}

func (t *terminal) system(c *Config) error {
	var err error
	if c.PollInterval, err = t.integer("Polling interval (seconds)", c.PollInterval, 10, int(^uint(0)>>1)); err != nil {
		return err
	}
	if c.MaxRetries, err = t.integer("Max retries", c.MaxRetries, 1, int(^uint(0)>>1)); err != nil {
		return err
	}
	levels := []string{"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"}
	d := 1
	for i, v := range levels {
		if v == c.LogLevel {
			d = i
		}
	}
	n, err := t.choice("Log level", levels, d)
	if err != nil {
		return err
	}
	c.LogLevel = levels[n]
	on, err := t.yes("Enable global proxy for IMAP/HTTP?", c.Proxy != nil && c.Proxy.Enabled)
	if err != nil {
		return err
	}
	if !on {
		c.Proxy = nil
		return nil
	}
	p := Proxy{Enabled: true, Scheme: "socks5", Port: 1080}
	if c.Proxy != nil {
		p = *c.Proxy
		p.Enabled = true
	}
	schemes := []string{"socks5", "socks4", "http"}
	d = 0
	for i, v := range schemes {
		if v == p.Scheme {
			d = i
		}
	}
	n, err = t.choice("Proxy scheme", schemes, d)
	if err != nil {
		return err
	}
	p.Scheme = schemes[n]
	if p.Host, err = t.required("Proxy host", p.Host, false); err != nil {
		return err
	}
	if p.Port, err = t.integer("Proxy port", p.Port, 1, 65535); err != nil {
		return err
	}
	if p.Username, err = t.ask("Proxy username (optional)", p.Username, false); err != nil {
		return err
	}
	if p.Password, err = t.ask("Proxy password (optional)", p.Password, true); err != nil {
		return err
	}
	c.Proxy = &p
	return nil
}

func (t *terminal) testConnection(ctx context.Context, c Config) error {
	n, err := NewNetwork(c.Proxy, time.Duration(c.NetworkTimeout)*time.Second)
	if err != nil {
		return err
	}
	defer n.Close()
	var tg *Telegram
	for _, nc := range c.Notifiers {
		if nc.Telegram != nil {
			tg = &Telegram{config: *nc.Telegram, client: n.Client(time.Duration(nc.Telegram.Timeout) * time.Second)}
			break
		}
	}
	if tg == nil {
		return fmt.Errorf("no Telegram bot configured")
	}
	if err = tg.call(ctx, "getMe", map[string]any{}, nil); err != nil {
		return err
	}
	fmt.Fprintln(t.out, "Success: Bot is reachable.")
	yes, err := t.yes("Send test message?", true)
	if err != nil {
		return err
	}
	if !yes {
		return nil
	}
	err = tg.send(ctx, previewText(Email{Account: "Test", Subject: "MailBot Test Message", Sender: "mailbot@local", Date: time.Now(), Body: "If you see this, the bot is working correctly."}), nil)
	if err == nil {
		fmt.Fprintln(t.out, "Test message sent.")
	}
	return err
}
