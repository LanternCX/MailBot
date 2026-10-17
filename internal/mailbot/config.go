package mailbot

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	PollInterval    int          `json:"poll_interval"`
	MaxRetries      int          `json:"max_retries"`
	LogLevel        string       `json:"log_level"`
	StatePath       string       `json:"state_path"`
	NetworkTimeout  int          `json:"network_timeout"`
	UnreadOnly      bool         `json:"unread_only"`
	MarkSeen        bool         `json:"mark_seen"`
	MaxMessageBytes int64        `json:"max_message_bytes,omitempty"`
	Accounts        []Account    `json:"accounts"`
	Notifiers       []Notifier   `json:"notifiers"`
	Proxy           *Proxy       `json:"proxy"`
	AI              AIConfig     `json:"ai"`
	Reader          ReaderConfig `json:"reader"`
}

type ReaderConfig struct {
	Listen    string `json:"listen"`
	PublicURL string `json:"public_url"`
}

func (r ReaderConfig) validate() error {
	if r.PublicURL == "" && r.Listen == "" {
		return nil
	}
	u, err := url.Parse(r.PublicURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.EscapedPath() != "" && u.EscapedPath() != "/") {
		return fmt.Errorf("reader.public_url must be an HTTPS origin, e.g. https://mail.example.com")
	}
	_, port, err := net.SplitHostPort(r.Listen)
	if err != nil {
		return fmt.Errorf("reader.listen must be host:port")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("reader.listen requires a port between 1 and 65535")
	}
	return nil
}

type Account struct {
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Host     string   `json:"imap_host"`
	Port     int      `json:"imap_port"`
	SSL      bool     `json:"use_ssl"`
	Folders  []string `json:"folders"`
	Enabled  bool     `json:"enabled"`
	WebURL   string   `json:"web_url"`
}

func (a *Account) UnmarshalJSON(b []byte) error {
	type plain Account
	v := plain{Port: 993, SSL: true, Enabled: true, Folders: []string{"INBOX"}}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*a = Account(v)
	return nil
}

type Notifier struct {
	Type     string          `json:"type"`
	Enabled  bool            `json:"enabled"`
	Telegram *TelegramConfig `json:"telegram"`
}

func (n *Notifier) UnmarshalJSON(b []byte) error {
	type plain Notifier
	v := plain{Enabled: true}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*n = Notifier(v)
	return nil
}

type TelegramConfig struct {
	Token   string `json:"bot_token"`
	ChatID  string `json:"chat_id"`
	APIBase string `json:"api_base"`
	Timeout int    `json:"timeout"`
}

func (t *TelegramConfig) UnmarshalJSON(b []byte) error {
	type plain TelegramConfig
	v := plain{APIBase: "https://api.telegram.org", Timeout: 30}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*t = TelegramConfig(v)
	return nil
}

type Proxy struct {
	Enabled  bool   `json:"enabled"`
	Scheme   string `json:"scheme"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (p *Proxy) UnmarshalJSON(b []byte) error {
	type plain Proxy
	v := plain{Enabled: true, Scheme: "socks5"}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*p = Proxy(v)
	return nil
}

type AIConfig struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
	Mode     string `json:"default_mode"`
	Language string `json:"language"`
}

func LoadConfig(path string) (Config, error) {
	c := DefaultConfig()
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if err = c.Reader.validate(); err != nil {
		return c, err
	}
	if c.PollInterval < 10 || c.MaxRetries < 1 || c.NetworkTimeout < 1 || c.MaxMessageBytes < 0 {
		return c, fmt.Errorf("poll_interval >= 10, max_retries >= 1 and network_timeout >= 1 required")
	}
	if c.StatePath == "" {
		c.StatePath = "mailbot.sqlite3"
	}
	if !filepath.IsAbs(c.StatePath) {
		c.StatePath = filepath.Join(filepath.Dir(path), c.StatePath)
	}
	// Runtime paths must be stable when a loaded configuration is saved again.
	// Otherwise configs/state.sqlite3 becomes configs/configs/state.sqlite3.
	c.StatePath, err = filepath.Abs(c.StatePath)
	if err != nil {
		return c, err
	}
	seen := map[string]bool{}
	for _, a := range c.Accounts {
		if !a.Enabled {
			continue
		}
		if a.Name == "" || a.Email == "" || a.Password == "" || a.Host == "" || a.Port < 1 || a.Port > 65535 || len(a.Folders) == 0 {
			return c, fmt.Errorf("invalid account %q", a.Name)
		}
		key := accountID(a)
		if seen[key] {
			return c, fmt.Errorf("duplicate account %q", a.Name)
		}
		seen[key] = true
	}
	for i := range c.Notifiers {
		n := &c.Notifiers[i]
		if !n.Enabled {
			continue
		}
		if n.Type != "telegram" || n.Telegram == nil || n.Telegram.Token == "" || n.Telegram.ChatID == "" {
			return c, fmt.Errorf("invalid notifier %d", i)
		}
		if n.Telegram.APIBase == "" {
			n.Telegram.APIBase = "https://api.telegram.org"
		}
		if n.Telegram.Timeout == 0 {
			n.Telegram.Timeout = 30
		}
		if n.Telegram.Timeout < 1 {
			return c, fmt.Errorf("invalid Telegram timeout")
		}
	}
	c.LogLevel = strings.ToUpper(c.LogLevel)
	switch c.LogLevel {
	case "DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL":
	default:
		return c, fmt.Errorf("invalid log_level")
	}
	if c.AI.Mode != "raw" && c.AI.Mode != "hybrid" && c.AI.Mode != "agent" {
		return c, fmt.Errorf("invalid AI mode")
	}
	if c.AI.Enabled {
		if _, err := buildAIRequest(c.AI, "", ""); err != nil {
			return c, err
		}
	}
	if c.Proxy != nil && c.Proxy.Enabled {
		p := c.Proxy
		if p.Host == "" || p.Port < 1 || p.Port > 65535 {
			return c, fmt.Errorf("invalid proxy address")
		}
		if p.Scheme != "socks5" && p.Scheme != "http" && p.Scheme != "socks4" {
			return c, fmt.Errorf("unsupported proxy scheme %q", p.Scheme)
		}
	}
	return c, nil
}

func DefaultConfig() Config {
	return Config{PollInterval: 60, MaxRetries: 3, LogLevel: "INFO", NetworkTimeout: 30, UnreadOnly: true, MarkSeen: true, StatePath: "mailbot.sqlite3", Accounts: []Account{}, Notifiers: []Notifier{}, AI: AIConfig{Provider: "deepseek", Model: "deepseek-chat", Mode: "hybrid", Language: "auto"}}
}

// SaveConfig atomically replaces configuration with owner-only permissions.
func SaveConfig(path string, c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(b, '\n'))
}

func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".mailbot-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// UpdateAISettings preserves unrelated and future configuration fields.
func UpdateAISettings(path string, values map[string]string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c map[string]json.RawMessage
	if err = json.Unmarshal(b, &c); err != nil {
		return err
	}
	ai := map[string]any{}
	if raw := c["ai"]; len(raw) > 0 {
		if err = json.Unmarshal(raw, &ai); err != nil {
			return err
		}
	}
	if ai == nil {
		ai = map[string]any{}
	}
	for key, value := range values {
		ai[key] = value
	}
	c["ai"], err = json.Marshal(ai)
	if err != nil {
		return err
	}
	b, err = json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(b, '\n'))
}

func accountID(a Account) string {
	return strings.ToLower(a.Host) + ":" + fmt.Sprint(a.Port) + ":" + a.Email
}
