package mailbot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-imap/server"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestQueueSurvivesRestartAndTracksEachTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	e := Email{ID: emailID("account", "INBOX", 1, 7), Subject: "test", UID: 7}
	if err = s.Commit("account", "INBOX", 1, 7, []Email{e}, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	d, err := s.Next("a")
	if err != nil || d == nil {
		t.Fatalf("delivery: %v %v", d, err)
	}
	if err = s.Done(d.ID); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if d, err = s.Next("a"); err != nil || d != nil {
		t.Fatalf("completed target resent: %v %v", d, err)
	}
	if d, err = s.Next("b"); err != nil || d == nil || d.Email.Subject != "test" {
		t.Fatalf("lost pending delivery: %v %v", d, err)
	}
	v, u, ok, err := s.Cursor("account", "INBOX")
	if err != nil || !ok || v != 1 || u != 7 {
		t.Fatalf("lost cursor: %d %d %v %v", v, u, ok, err)
	}
	if err = s.Commit("account", "INBOX", 1, 7, []Email{e}, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	n, _ := s.Pending()
	if n != 1 {
		t.Fatalf("duplicate enqueue: %d", n)
	}
}

func TestIdentityIncludesAccountFolderAndValidity(t *testing.T) {
	ids := map[string]bool{}
	for _, id := range []string{emailID("a", "INBOX", 1, 7), emailID("b", "INBOX", 1, 7), emailID("a", "Spam", 1, 7), emailID("a", "INBOX", 2, 7)} {
		if ids[id] {
			t.Fatal("identity collision")
		}
		ids[id] = true
		if len("translate:"+id) > 64 {
			t.Fatal("callback too long")
		}
	}
}

func TestDeliveryRetriesFailedPartWithoutResendingSuccessfulPart(t *testing.T) {
	s := &Service{store: testStore(t), config: Config{}, aiSlots: make(chan struct{}, 2)}
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 2 {
			w.WriteHeader(429)
			io.WriteString(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":12}}`)
			return
		}
		io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer api.Close()
	tg := &Telegram{config: TelegramConfig{APIBase: api.URL, Token: "secret", ChatID: "1"}, client: api.Client()}
	e := Email{ID: "mail", Subject: strings.Repeat("a", 4000)}
	if err := s.store.Commit("a", "INBOX", 1, 1, []Email{e}, []string{"target"}); err != nil {
		t.Fatal(err)
	}
	d, _ := s.store.Next("target")
	err := s.deliver(context.Background(), tg, d)
	var limit *APIError
	if !errors.As(err, &limit) || limit.RetryAfter != 12 {
		t.Fatalf("missing rate limit: %v", err)
	}
	d, _ = s.store.Next("target")
	if d.Part != 1 {
		t.Fatalf("missing partial progress: %d", d.Part)
	}
	if err = s.deliver(context.Background(), tg, d); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("resent completed part: %d calls", calls.Load())
	}
}

func TestTargetRateLimitSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Commit("account", "INBOX", 1, 2, []Email{{ID: "first"}, {ID: "second"}}, []string{"limited", "healthy"}); err != nil {
		t.Fatal(err)
	}
	d, err := store.Next("limited")
	if err != nil || d == nil {
		t.Fatalf("missing queued delivery %v", err)
	}
	if err := store.RetryLimited(d.ID, "limited", time.Hour); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if d, err := store.Next("limited"); err != nil || d != nil {
		t.Fatalf("target limit lost after restart: %+v %v", d, err)
	}
	if d, err := store.Next("healthy"); err != nil || d == nil {
		t.Fatalf("another target blocked: %+v %v", d, err)
	}
	if _, err := store.db.Exec(`UPDATE target_limits SET until=0`); err != nil {
		t.Fatal(err)
	}
	if d, err := store.Next("limited"); err != nil || d == nil || d.Email.ID != "second" {
		t.Fatalf("target did not resume after cooldown: %+v %v", d, err)
	}
}

func TestIMAPBaselineThenCatchUpAfterRestart(t *testing.T) {
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
	c.Timeout = 2 * time.Second
	if err = c.Login("username", "password"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{store: store, targets: []string{"target"}}
	a := Account{Name: "test", Email: "username", Host: "localhost", Port: 993}
	if err = s.pollFolder(c, a, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.Pending(); n != 0 {
		t.Fatal("forwarded historical mail")
	}
	store.Close()
	// Mail arrives while the service is stopped, and is already read elsewhere.
	body := "From: sender@example.org\r\nSubject: Offline arrival\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nhello"
	if err = c.Append("INBOX", []string{imap.SeenFlag}, time.Now(), strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s.store = store
	if err = s.pollFolder(c, a, "INBOX"); err != nil {
		t.Fatal(err)
	}
	d, err := store.Next("target")
	if err != nil || d == nil || d.Email.Subject != "Offline arrival" {
		t.Fatalf("offline mail missing: %v %v", d, err)
	}
	if err = s.pollFolder(c, a, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.Pending(); n != 1 {
		t.Fatalf("duplicate mail: %d", n)
	}
}

func TestUnavailableFolderDoesNotBlockOtherFolders(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(memory.New())
	srv.AllowInsecureAuth = true
	go srv.Serve(listener)
	defer srv.Close()
	admin, err := client.Dial(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Terminate()
	if err := admin.Login("username", "password"); err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	var pn int
	json.Unmarshal([]byte(port), &pn)
	n, _ := NewNetwork(nil, 2*time.Second)
	defer n.Close()
	s := &Service{store: testStore(t), network: n, timeout: 2 * time.Second, targets: []string{"target"}}
	a := Account{Name: "test", Email: "username", Password: "password", Host: host, Port: pn, Folders: []string{"missing-folder", "INBOX"}}
	if err := s.pollAccount(context.Background(), a); err == nil || !strings.Contains(err.Error(), "missing-folder") {
		t.Fatalf("folder error not reported: %v", err)
	}
	if _, _, exists, err := s.store.Cursor(accountID(a), "INBOX"); err != nil || !exists {
		t.Fatalf("healthy folder not baselined: %v", err)
	}
	body := "From: sender@example.org\r\nSubject: Healthy folder\r\n\r\nmessage"
	if err := admin.Append("INBOX", nil, time.Now(), strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if err := s.pollAccount(context.Background(), a); err == nil {
		t.Fatal("missing folder error disappeared")
	}
	d, err := s.store.Next("target")
	if err != nil || d == nil || d.Email.Subject != "Healthy folder" {
		t.Fatalf("healthy folder mail blocked: %+v %v", d, err)
	}
}

func TestStalledIMAPConnectionCancelsPromptly(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	n, _ := NewNetwork(nil, 30*time.Second)
	defer n.Close()
	s := &Service{network: n, timeout: 30 * time.Second}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	var pn int
	json.Unmarshal([]byte(port), &pn)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.pollAccount(ctx, Account{Host: host, Port: pn}) }()
	conn := <-accepted
	defer conn.Close()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancelled greeting")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown stuck on IMAP greeting")
	}
}

func TestUnauthorizedChatCannotChangeSettings(t *testing.T) {
	s := &Service{store: testStore(t), config: Config{AI: AIConfig{Enabled: true}}, telegrams: []*Telegram{{config: TelegramConfig{Token: "token", ChatID: "1"}}}}
	u := update{Message: &message{Text: "/mode agent", Chat: chat{ID: "2"}}}
	if err := s.handleUpdate(context.Background(), s.telegrams[0], u, make(chan func(), 1)); err != nil {
		t.Fatal(err)
	}
	if s.store.Setting("mode", "") != "" {
		t.Fatal("unauthorized settings change")
	}
}

func TestConfigDefaultsAndRelativeState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"accounts":[{"name":"test","email":"test","password":"secret","imap_host":"imap.test"}],"notifiers":[{"type":"telegram","telegram":{"bot_token":"secret","chat_id":"1"}}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Accounts[0].SSL || !c.Accounts[0].Enabled || c.Notifiers[0].Telegram.Timeout != 30 || filepath.Dir(c.StatePath) != filepath.Dir(path) {
		t.Fatal("configuration defaults incompatible")
	}
}

func TestHTMLBodyRetainsBlockBoundariesAndNormalizesWhitespace(t *testing.T) {
	text := htmlText(`<html><head><title>hidden title</title></head><body><p>First   paragraph</p>Following text<br>Next line<div>Another block</div><script>hidden script</script><style>hidden style</style></body></html>`)
	if strings.Contains(text, "hidden") || strings.Contains(text, "   ") || strings.Contains(text, "\n\n\n") {
		t.Fatalf("HTML was not cleaned: %q", text)
	}
	if !strings.Contains(text, "First paragraph \nFollowing text") || !strings.Contains(text, "Following text \nNext line") {
		t.Fatalf("block or line boundary lost: %q", text)
	}
}

func TestUnknownCharsetRetainsReadableBody(t *testing.T) {
	for _, body := range []string{
		"Content-Type: text/plain; charset=x-unknown\r\n\r\nReadable 中文",
		"Content-Type: multipart/mixed; boundary=fixture\r\n\r\n--fixture\r\nContent-Type: text/plain; charset=x-unknown\r\n\r\nReadable 中文\r\n--fixture--\r\n",
	} {
		var e Email
		if err := parseEmail(strings.NewReader("From: sender@example.org\r\nSubject: test\r\n"+body), &e); err != nil || e.Body != "Readable 中文" {
			t.Fatalf("readable body discarded: %+v %v", e, err)
		}
	}
}

func TestMimeAndHTMLParsing(t *testing.T) {
	raw := "From: sender@example.org\r\nSubject: =?UTF-8?B?5rWL6K+V?=\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<head><style>hidden</style></head><p>Hello<br>世界</p><script>bad()</script>"
	var e Email
	if err := parseEmail(strings.NewReader(raw), &e); err != nil {
		t.Fatal(err)
	}
	if e.Subject != "测试" || !strings.Contains(e.Body, "世界") || strings.Contains(e.Body, "bad()") {
		t.Fatalf("bad MIME decode: %+v", e)
	}
}

func TestOnlyOneServiceCanOwnState(t *testing.T) {
	s := &Service{store: testStore(t)}
	if err := s.acquireLock(); err != nil {
		t.Fatal(err)
	}
	defer s.lock.Close()
	other := &Service{store: s.store}
	if err := other.acquireLock(); err == nil {
		t.Fatal("second service acquired live process lock")
	}
}

func TestAgentFallsBackToOriginalWhenAIIsUnavailable(t *testing.T) {
	var sent string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			w.WriteHeader(503)
			return
		}
		var payload struct {
			Text string `json:"text"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		sent = payload.Text
		io.WriteString(w, `{"ok":true,"result":{}}`)
	}))
	defer api.Close()
	n, _ := NewNetwork(nil, time.Second)
	defer n.Close()
	s := &Service{store: testStore(t), network: n, config: Config{AI: AIConfig{Enabled: true, Mode: "agent", BaseURL: api.URL, Model: "test"}}, configDir: t.TempDir(), aiSlots: make(chan struct{}, 2)}
	e := Email{ID: "fallback", Subject: "Keep this mail", Body: "original content"}
	if err := s.store.Commit("a", "INBOX", 1, 1, []Email{e}, []string{"target"}); err != nil {
		t.Fatal(err)
	}
	d, _ := s.store.Next("target")
	tg := &Telegram{config: TelegramConfig{APIBase: api.URL, Token: "secret", ChatID: "1"}, client: api.Client()}
	if err := s.deliver(context.Background(), tg, d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sent, "original content") {
		t.Fatalf("lost mail on AI failure: %q", sent)
	}
}

func TestHTTPProxyTunnelsWithoutLocalDestinationDNS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			result <- err
			return
		}
		if req.Method != "CONNECT" || req.Host != "unresolvable.invalid:993" {
			result <- errors.New("wrong tunnel destination")
			return
		}
		io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\nhello")
		result <- nil
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	var pn int
	json.Unmarshal([]byte(port), &pn)
	n, err := NewNetwork(&Proxy{Enabled: true, Scheme: "http", Host: host, Port: pn}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	conn, err := n.dial(context.Background(), "tcp", "unresolvable.invalid:993")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	b := make([]byte, 5)
	if _, err = io.ReadFull(conn, b); err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello" {
		t.Fatal("lost buffered tunnel bytes")
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
}

func TestServiceShutdownCancelsLongPollAndReleasesLock(t *testing.T) {
	started := make(chan struct{}, 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if strings.HasSuffix(r.URL.Path, "/setMyCommands") {
			io.WriteString(w, `{"ok":true,"result":true}`)
			return
		}
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer api.Close()
	c := Config{PollInterval: 60, NetworkTimeout: 30, StatePath: filepath.Join(t.TempDir(), "state.sqlite3"), Notifiers: []Notifier{{Enabled: true, Type: "telegram", Telegram: &TelegramConfig{Token: "token", ChatID: "1", APIBase: api.URL, Timeout: 30}}}}
	s, err := NewService(c, filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("poll did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("service shutdown hung")
	}
	store, err := OpenStore(c.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	other := &Service{store: store}
	if err = other.acquireLock(); err != nil {
		t.Fatal("process lock not released:", err)
	}
	other.lock.Close()
}

func TestQueueFailureDoesNotAdvanceCursor(t *testing.T) {
	s := testStore(t)
	if err := s.Baseline("a", "INBOX", 1, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_delivery BEFORE INSERT ON deliveries BEGIN SELECT RAISE(ABORT,'disk failure simulation'); END`); err != nil {
		t.Fatal(err)
	}
	err := s.Commit("a", "INBOX", 1, 6, []Email{{ID: "test"}}, []string{"target"})
	if err == nil {
		t.Fatal("expected simulated storage failure")
	}
	_, uid, _, err := s.Cursor("a", "INBOX")
	if err != nil || uid != 5 {
		t.Fatalf("advanced cursor without durable mail: %d %v", uid, err)
	}
	if _, err = s.Email("test"); err == nil {
		t.Fatal("partial transaction committed")
	}
}
