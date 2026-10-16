package mailbot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

type Service struct {
	config       Config
	configDir    string
	configPath   string
	configMu     sync.Mutex
	store        *Store
	network      *Network
	timeout      time.Duration
	targets      []string
	telegrams    []*Telegram
	mu           sync.RWMutex
	rulesMu      sync.Mutex
	rulesPending map[string]string
	status       map[string]string
	lastPoll     map[string]time.Time
	lastSend     map[string]time.Time
	aiSlots      chan struct{}
	lock         *flock.Flock
}

func NewService(c Config, configPath string) (*Service, error) {
	enabled := 0
	for _, n := range c.Notifiers {
		if n.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return nil, fmt.Errorf("configure an enabled Telegram notifier first")
	}
	n, err := NewNetwork(c.Proxy, time.Duration(c.NetworkTimeout)*time.Second)
	if err != nil {
		return nil, err
	}
	store, err := OpenStore(c.StatePath)
	if err != nil {
		n.Close()
		return nil, err
	}
	s := &Service{config: c, configPath: configPath, configDir: filepath.Dir(configPath), store: store, network: n, timeout: time.Duration(c.NetworkTimeout) * time.Second, status: map[string]string{}, lastPoll: map[string]time.Time{}, lastSend: map[string]time.Time{}, aiSlots: make(chan struct{}, 2), rulesPending: map[string]string{}}
	seen := map[string]bool{}
	for _, nc := range c.Notifiers {
		if !nc.Enabled {
			continue
		}
		tc := *nc.Telegram
		h := sha256.Sum256([]byte(tc.Token + "\x00" + tc.ChatID))
		key := hex.EncodeToString(h[:16])
		if seen[key] {
			continue
		}
		seen[key] = true
		s.telegrams = append(s.telegrams, &Telegram{config: tc, key: key, client: n.Client(time.Duration(tc.Timeout) * time.Second)})
		s.targets = append(s.targets, key)
	}
	return s, nil
}

func wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func backoff(attempt int) time.Duration {
	if attempt > 8 {
		attempt = 8
	}
	return time.Duration(1<<attempt) * time.Second
}

func (s *Service) Run(ctx context.Context) error {
	defer s.store.Close()
	defer s.network.Close()
	// An exclusive process lock prevents two consumers from sending the same queue.
	if err := s.acquireLock(); err != nil {
		return err
	}
	defer s.lock.Close()
	if err := s.initializeSettings(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	failures := make(chan error, 1)
	start := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
			if ctx.Err() == nil {
				select {
				case failures <- fmt.Errorf("background worker stopped unexpectedly"):
				default:
				}
				cancel()
			}
		}()
	}
	for _, a := range s.config.Accounts {
		if !a.Enabled {
			continue
		}
		start(func() { s.accountLoop(ctx, a) })
	}
	for _, t := range s.telegrams {
		start(func() { s.deliveryLoop(ctx, t) })
	}
	// Telegram permits only one getUpdates consumer per bot token.
	seen := map[string]bool{}
	for _, t := range s.telegrams {
		if !seen[t.config.Token] {
			seen[t.config.Token] = true
			start(func() { s.botLoop(ctx, t) })
		}
	}
	start(func() {
		for wait(ctx, time.Minute) {
			slog.Info("service health", "status", s.Status())
		}
	})
	start(func() {
		for {
			if err := s.store.Cleanup(); err != nil {
				slog.Error("state cleanup failed", "error", err)
			}
			if !wait(ctx, time.Hour) {
				return
			}
		}
	})
	slog.Info("MailBot started", "accounts", len(s.config.Accounts), "targets", len(s.targets))
	<-ctx.Done()
	wg.Wait()
	slog.Info("MailBot stopped")
	select {
	case err := <-failures:
		return err
	default:
	}
	return nil
}

func (s *Service) acquireLock() error {
	if s.lock != nil {
		return fmt.Errorf("service already owns the state database")
	}
	lock := flock.New(s.store.path+".lock", flock.SetPermissions(0600))
	ok, err := lock.TryLock()
	if err != nil {
		lock.Close()
		return err
	}
	if !ok {
		lock.Close()
		return fmt.Errorf("another MailBot owns this state database")
	}
	s.lock = lock
	return nil
}
func (s *Service) initializeSettings() error {
	// Only the lock owner may initialize runtime settings from the authoritative JSON.
	mode := s.config.AI.Mode
	if !s.config.AI.Enabled {
		mode = "raw"
	}
	if err := s.store.Set("mode", mode); err != nil {
		return err
	}
	return s.store.Set("language", s.config.AI.Language)
}

func (s *Service) accountLoop(ctx context.Context, a Account) {
	failures := 0
	for ctx.Err() == nil {
		err := s.pollAccount(ctx, a)
		s.mu.Lock()
		if err != nil {
			s.status[a.Name] = "error"
		} else {
			s.status[a.Name] = "ok"
			s.lastPoll[a.Name] = time.Now()
		}
		s.mu.Unlock()
		delay := time.Duration(s.config.PollInterval) * time.Second
		if err != nil && ctx.Err() == nil {
			failures++
			slog.Warn("account poll failed; reconnecting", "account", a.Name, "error", err)
			delay = backoff(failures + 2)
			if failures > s.config.MaxRetries && delay < time.Duration(s.config.PollInterval)*time.Second {
				delay = time.Duration(s.config.PollInterval) * time.Second
			}
		} else {
			failures = 0
		}
		if !wait(ctx, delay) {
			return
		}
	}
}

func (s *Service) mode() string {
	fallback := s.config.AI.Mode
	if fallback == "" || !s.config.AI.Enabled {
		fallback = "raw"
	}
	return s.store.Setting("mode", fallback)
}

func (s *Service) deliveryLoop(ctx context.Context, t *Telegram) {
	for ctx.Err() == nil {
		d, err := s.store.Next(t.key)
		if err != nil {
			slog.Error("queue read failed", "error", err)
			if !wait(ctx, 5*time.Second) {
				return
			}
			continue
		}
		if d == nil {
			if !wait(ctx, time.Second) {
				return
			}
			continue
		}
		err = s.deliver(ctx, t, d)
		if err == nil {
			if err = s.store.Done(d.ID); err != nil {
				slog.Error("delivery checkpoint failed", "error", err)
				if !wait(ctx, 5*time.Second) {
					return
				}
			}
			s.mu.Lock()
			s.lastSend[t.key] = time.Now()
			s.mu.Unlock()
			slog.Info("email forwarded", "email_id", d.Email.ID)
		} else if ctx.Err() == nil {
			delay := backoff(d.Attempts + 2)
			var api *APIError
			if errors.As(err, &api) && api.RetryAfter > 0 {
				delay = time.Duration(api.RetryAfter) * time.Second
			}
			var retryErr error
			if api != nil && api.Code == 429 {
				retryErr = s.store.RetryLimited(d.ID, t.key, delay)
			} else {
				retryErr = s.store.Retry(d.ID, delay)
			}
			if retryErr != nil {
				slog.Error("retry checkpoint failed", "error", retryErr)
			}
			slog.Warn("delivery failed; retained for retry", "email_id", d.Email.ID, "error", err, "retry_in", delay)
			// Apply rate limits to the target, not just this email.
			if !wait(ctx, time.Second) {
				return
			}
			if api != nil && api.Code == 429 {
				if !wait(ctx, delay) {
					return
				}
			}
		}
		if !wait(ctx, time.Second) {
			return
		}
	}
}

func (s *Service) deliver(ctx context.Context, t *Telegram, d *Delivery) error {
	mode := s.mode()
	if d.Text == "" {
		card := Card{Text: rawText(d.Email)}
		language := s.store.Setting("language", s.config.AI.Language)
		if mode == "hybrid" {
			card = hybridCard(d.Email, language)
		}
		if mode == "agent" {
			select {
			case s.aiSlots <- struct{}{}:
				a, err := s.analyze(ctx, d.Email)
				<-s.aiSlots
				if err == nil {
					card = summaryCard(a)
					card.Buttons = mailButtons(d.Email, "agent", a.SourceLanguage, language)
				} else {
					slog.Warn("AI failed; forwarding original", "email_id", d.Email.ID)
					card.Buttons = mailButtons(d.Email, "agent", detectLanguage(d.Email.Body), language)
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		d.Text = encodeCard(card)
		if err := s.store.SaveText(d.ID, d.Text); err != nil {
			return err
		}
	}
	card := decodeCard(d.Text)
	parts := card.Parts()
	for i := d.Part; i < len(parts); i++ {
		partCard := card
		if i < len(parts)-1 {
			partCard.Buttons = nil
		}
		if err := t.sendPart(ctx, parts[i], partCard, 0); err != nil {
			return err
		}
		if err := s.store.SavePart(d.ID, i+1); err != nil {
			return err
		}
		if i < len(parts)-1 && !wait(ctx, time.Second) {
			return ctx.Err()
		}
	}
	return nil
}
