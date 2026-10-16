package mailbot

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-imap/server"
)

type idleTestExtension struct {
	events   chan server.Conn
	disabled bool
}

func (e *idleTestExtension) Capabilities(server.Conn) []string { return nil }
func (e *idleTestExtension) NewConn(c server.Conn) server.Conn {
	return idleTestConn{Conn: c, extension: e}
}

type idleTestConn struct {
	server.Conn
	extension *idleTestExtension
}

func (c idleTestConn) Capabilities() []string {
	var caps []string
	for _, cap := range c.Conn.Capabilities() {
		if cap != "IDLE" || !c.extension.disabled {
			caps = append(caps, cap)
		}
	}
	return caps
}
func (e *idleTestExtension) Command(name string) server.HandlerFactory {
	return nil
}

func (c idleTestConn) Read(p []byte) (int, error) {
	// The built-in IDLE handler reads DONE through Conn after sending its
	// continuation; command parsing otherwise uses the server's own reader.
	if c.Context().Mailbox != nil {
		select {
		case c.extension.events <- c:
		case <-c.Context().LoggedOut:
			return 0, io.EOF
		}
	}
	return c.Conn.Read(p)
}

type countedBackend struct {
	backend.Backend
	logins atomic.Int32
}

func (b *countedBackend) Login(info *imap.ConnInfo, user, password string) (backend.User, error) {
	b.logins.Add(1)
	return b.Backend.Login(info, user, password)
}

func TestIDLEDeliversWithoutPollingAndCatchesUpAfterDisconnect(t *testing.T) {
	ext := &idleTestExtension{events: make(chan server.Conn, 8)}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(memory.New())
	srv.AllowInsecureAuth = true
	srv.Enable(ext)
	go srv.Serve(listener)
	defer srv.Close()
	admin, err := client.Dial(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Terminate()
	if err = admin.Login("username", "password"); err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"ok":true,"result":[]}`) }))
	defer api.Close()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	pn, _ := strconv.Atoi(port)
	c := DefaultConfig()
	c.PollInterval = 600
	c.NetworkTimeout = 1
	c.StatePath = filepath.Join(t.TempDir(), "state.sqlite3")
	a := Account{Name: "Test", Email: "username", Password: "password", Host: host, Port: pn, Enabled: true, Folders: []string{"INBOX"}}
	c.Accounts = []Account{a}
	c.Notifiers = []Notifier{{Type: "telegram", Enabled: true, Telegram: &TelegramConfig{Token: "token", ChatID: "42", APIBase: api.URL, Timeout: 1}}}
	s, err := NewService(c, filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.store.Close()
	defer s.network.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.accountLoop(ctx, a) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("IDLE shutdown stalled")
		}
	}()
	nextIdle := func() server.Conn {
		t.Helper()
		select {
		case conn := <-ext.events:
			return conn
		case <-time.After(12 * time.Second):
			t.Fatal("did not enter IDLE")
			return nil
		}
	}
	conn := nextIdle()
	appendMail := func(subject string) {
		t.Helper()
		if err := admin.Append("INBOX", nil, time.Now(), strings.NewReader("Subject: "+subject+"\r\n\r\nmessage")); err != nil {
			t.Fatal(err)
		}
	}
	appendMail("pushed immediately")
	if err = conn.WriteResp(imap.NewUntaggedResp([]interface{}{uint32(2), imap.RawString("EXISTS")})); err != nil {
		t.Fatal(err)
	}
	eventually(t, 2*time.Second, func() bool { n, err := s.store.Pending(); return err == nil && n == 1 })
	conn = nextIdle()
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	appendMail("arrived while disconnected")
	nextIdle()
	eventually(t, time.Second, func() bool { n, err := s.store.Pending(); return err == nil && n == 2 })
	_, uid, _, err := s.store.Cursor(accountID(a), "INBOX")
	if err != nil || uid != 8 {
		t.Fatalf("catch-up cursor: %d %v", uid, err)
	}
}

func TestUnsupportedIDLEPollsUsingOneConnection(t *testing.T) {
	be := &countedBackend{Backend: memory.New()}
	ext := &idleTestExtension{events: make(chan server.Conn, 2), disabled: true}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(be)
	srv.AllowInsecureAuth = true
	srv.Enable(ext)
	go srv.Serve(listener)
	defer srv.Close()
	admin, err := client.Dial(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Terminate()
	if err = admin.Login("username", "password"); err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	pn, _ := strconv.Atoi(port)
	network, err := NewNetwork(nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer network.Close()
	a := Account{Name: "Test", Email: "username", Password: "password", Host: host, Port: pn, Folders: []string{"INBOX"}}
	s := &Service{config: DefaultConfig(), network: network, timeout: time.Second, store: testStore(t), targets: []string{"target"}, status: map[string]string{}, lastPoll: map[string]time.Time{}}
	s.config.PollInterval = 1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.watchFolder(ctx, a) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("fallback shutdown stalled")
		}
	}()
	eventually(t, time.Second, func() bool { _, _, exists, _ := s.store.Cursor(accountID(a), "INBOX"); return exists })
	if err = admin.Append("INBOX", nil, time.Now(), strings.NewReader("Subject: fallback\r\n\r\nbody")); err != nil {
		t.Fatal(err)
	}
	eventually(t, 3*time.Second, func() bool { n, _ := s.store.Pending(); return n == 1 })
	if be.logins.Load() != 2 {
		t.Fatalf("unnecessary reconnects: %d logins", be.logins.Load())
	}
	if len(ext.events) != 0 {
		t.Fatal("used IDLE without server support")
	}
}

func TestIDLERenewsWithoutNotifications(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(memory.New())
	srv.AllowInsecureAuth = true
	go srv.Serve(listener)
	defer srv.Close()
	c, err := client.Dial(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Terminate()
	if err = c.Login("username", "password"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Select("INBOX", true); err != nil {
		t.Fatal(err)
	}
	s := &Service{timeout: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for range 2 {
		if err = s.idleMailbox(ctx, c, make(chan struct{}), 30*time.Millisecond); err != nil {
			t.Fatal(err)
		}
		// A scan must be able to issue commands after DONE, even without EXISTS.
		if _, err = c.UidSearch(imap.NewSearchCriteria()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVerificationMailBypassesBlockedAIAndPreservesFullCode(t *testing.T) {
	for _, mode := range []string{"raw", "hybrid", "agent"} {
		t.Run(mode, func(t *testing.T) {
			aiStarted := make(chan struct{}, 1)
			release := make(chan struct{})
			received := make(chan string, 8)
			var aiCalls atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/chat/completions") {
					aiCalls.Add(1)
					aiStarted <- struct{}{}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					io.WriteString(w, `{"choices":[{"message":{"content":"{\"summary\":\"normal summary\"}"}}]}`)
					return
				}
				var p map[string]any
				json.NewDecoder(r.Body).Decode(&p)
				if text, ok := p["text"].(string); ok {
					received <- text
				}
				io.WriteString(w, `{"ok":true,"result":true}`)
			}))
			defer api.Close()
			c := DefaultConfig()
			c.StatePath = filepath.Join(t.TempDir(), "queue.sqlite3")
			c.AI = AIConfig{Enabled: true, Provider: "deepseek", BaseURL: api.URL, Mode: mode, Language: "zh"}
			c.Notifiers = []Notifier{{Enabled: true, Type: "telegram", Telegram: &TelegramConfig{Token: "t", ChatID: "42", APIBase: api.URL, Timeout: 1}}}
			s, err := NewService(c, filepath.Join(t.TempDir(), "config.json"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.store.Close()
			defer s.network.Close()
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{}, 2)
			go func() { s.deliveryLoop(ctx, s.telegrams[0]); done <- struct{}{} }()
			go func() { s.deliveryLane(ctx, s.telegrams[0], true); done <- struct{}{} }()
			defer func() {
				cancel()
				close(release)
				for range 2 {
					select {
					case <-done:
					case <-time.After(2 * time.Second):
						t.Error("delivery worker did not stop")
					}
				}
			}()
			if mode == "agent" {
				e := Email{ID: "normal", Subject: "Newsletter", Body: "ordinary content"}
				if err = s.store.Commit("a", "INBOX", 1, 1, []Email{e}, s.targets); err != nil {
					t.Fatal(err)
				}
				select {
				case <-aiStarted:
				case <-time.After(2 * time.Second):
					t.Fatal("normal AI did not start")
				}
			}
			e := Email{ID: "otp", Subject: "Your verification code", Body: strings.Repeat("Introduction ", 20) + "Your code is 654321"}
			if err = s.store.Commit("a", "INBOX", 1, 2, []Email{e}, s.targets); err != nil {
				t.Fatal(err)
			}
			select {
			case text := <-received:
				if !strings.Contains(text, "654321") {
					t.Fatalf("code missing: %q", text)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("verification mail blocked by AI")
			}
			want := int32(0)
			if mode == "agent" {
				want = 1
			}
			if aiCalls.Load() != want {
				t.Fatalf("AI invoked for verification mail: %d calls", aiCalls.Load())
			}
		})
	}
}

func TestOldQueueMigrationRetainsProgressAndPromotesVerificationMail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite3")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE emails(id TEXT PRIMARY KEY,data BLOB NOT NULL,created INTEGER NOT NULL);
CREATE TABLE deliveries(id INTEGER PRIMARY KEY,email TEXT NOT NULL,target TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,next INTEGER NOT NULL DEFAULT 0,done INTEGER NOT NULL DEFAULT 0,part INTEGER NOT NULL DEFAULT 0,text TEXT NOT NULL DEFAULT '',UNIQUE(email,target));`)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range []Email{{ID: "ordinary", Subject: "Newsletter"}, {ID: "code", Subject: "验证码 123456"}} {
		data, _ := json.Marshal(e)
		if _, err = db.Exec(`INSERT INTO emails VALUES(?,?,?)`, e.ID, data, time.Now().Unix()); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO deliveries(email,target,attempts,part,text) VALUES(?,'target',3,1,'saved card')`, e.ID); err != nil {
			t.Fatal(i, err)
		}
	}
	db.Close()
	for iteration := range 2 {
		store, err := OpenStore(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := store.Next("target")
		if err != nil || d == nil || d.Email.ID != "code" || !d.Priority || d.Part != 1 || d.Attempts != 3+iteration || d.Text != "saved card" {
			t.Fatalf("migration lost state: %+v %v", d, err)
		}
		if err = store.RetryLimited(d.ID, "target", time.Minute); err != nil {
			t.Fatal(err)
		}
		if d, err = store.NextLane("target", false); err != nil || d != nil {
			t.Fatalf("normal lane ignored urgent cooldown: %+v %v", d, err)
		}
		if _, err = store.db.Exec(`UPDATE target_limits SET until=0;UPDATE deliveries SET next=0`); err != nil {
			t.Fatal(err)
		}
		store.Close()
	}
}

func TestVerificationRecognition(t *testing.T) {
	for _, tc := range []struct {
		subject, body string
		want          bool
	}{
		{"登录验证码", "123456", true},
		{"Sign-in code", "654321", true},
		{"Your ONE-TIME password", "123456", true},
		{"通知", strings.Repeat("前言", 100) + "验证码 123456", true},
		{"認証コード", "123456", true},
		{"Newsletter", "Order number 123456", false},
		{"Helicopter news", "Latest developments", false},
		{"Notice", strings.Repeat("x", 4000) + "verification code", false},
	} {
		if got := verificationMail(Email{Subject: tc.subject, Body: tc.body}); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.subject, got, tc.want)
		}
	}
}

func TestDeliveryLanesShareTelegramCooldown(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":2}}`)
			return
		}
		io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer api.Close()
	s := &Service{store: testStore(t)}
	tg := &Telegram{key: "shared", client: api.Client(), config: TelegramConfig{Token: "token", ChatID: "42", APIBase: api.URL}}
	if err := s.sendDeliveryPart(context.Background(), tg, &Delivery{}, "normal", Card{}); err == nil {
		t.Fatal("expected rate limit")
	}
	delay, err := s.store.TargetDelay(tg.key)
	if err != nil || delay <= 0 {
		t.Fatalf("missing durable cooldown: %v %v", delay, err)
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := s.sendDeliveryPart(ctx, tg, &Delivery{Priority: true}, "verification code", Card{}); err != nil {
		t.Fatal(err)
	}
	if time.Since(start)+50*time.Millisecond < delay || calls.Load() != 2 {
		t.Fatal("urgent delivery bypassed the shared cooldown")
	}
}
