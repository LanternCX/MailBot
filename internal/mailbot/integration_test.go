package mailbot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

func eventually(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestCompleteServiceFetchQueuesRetriesAndDelivers(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	imapServer := server.New(memory.New())
	imapServer.AllowInsecureAuth = true
	go imapServer.Serve(listener)
	defer imapServer.Close()
	admin, err := client.Dial(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Terminate()
	if err = admin.Login("username", "password"); err != nil {
		t.Fatal(err)
	}
	var sends, registrations atomic.Int32
	received := make(chan string, 4)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		switch {
		case strings.HasSuffix(r.URL.Path, "/setMyCommands"):
			registrations.Add(1)
			io.WriteString(w, `{"ok":true,"result":true}`)
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			if sends.Add(1) == 1 {
				w.WriteHeader(429)
				io.WriteString(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":1}}`)
				return
			}
			received <- payload["text"].(string)
			io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
		default:
			io.WriteString(w, `{"ok":true,"result":[]}`)
		}
	}))
	defer api.Close()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	pn, _ := strconv.Atoi(port)
	c := DefaultConfig()
	c.PollInterval = 1
	c.StatePath = filepath.Join(t.TempDir(), "state.sqlite3")
	c.Accounts = []Account{{Name: "Test", Email: "username", Password: "password", Host: host, Port: pn, SSL: false, Enabled: true, Folders: []string{"INBOX"}}}
	c.Notifiers = []Notifier{{Type: "telegram", Enabled: true, Telegram: &TelegramConfig{Token: "token", ChatID: "42", APIBase: api.URL, Timeout: 2}}}
	s, err := NewService(c, filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	eventually(t, 3*time.Second, func() bool { s.mu.RLock(); defer s.mu.RUnlock(); return !s.lastPoll["Test"].IsZero() })
	body := "From: Example <sender@example.org>\r\nSubject: End to end\r\nContent-Type: text/plain\r\n\r\nimportant email"
	if err = admin.Append("INBOX", nil, time.Now(), strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	select {
	case text := <-received:
		if !strings.Contains(text, "End to end") || !strings.Contains(text, "important email") {
			t.Fatalf("bad delivery: %q", text)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("email was not delivered")
	}
	eventually(t, 2*time.Second, func() bool { n, err := s.store.Pending(); return err == nil && n == 0 })
	if sends.Load() != 2 || registrations.Load() != 1 {
		t.Fatalf("retry or command registration failed: sends=%d registrations=%d", sends.Load(), registrations.Load())
	}
	cancel()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
}

func TestSOCKS4And5RouteAndAuthenticateWithoutLocalDNS(t *testing.T) {
	for _, scheme := range []string{"socks4", "socks5"} {
		t.Run(scheme, func(t *testing.T) {
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
				conn.SetDeadline(time.Now().Add(2 * time.Second))
				r := bufio.NewReader(conn)
				fail := func(err error) { result <- err }
				if scheme == "socks5" {
					head := make([]byte, 2)
					if _, err = io.ReadFull(r, head); err != nil {
						fail(err)
						return
					}
					methods := make([]byte, int(head[1]))
					io.ReadFull(r, methods)
					conn.Write([]byte{5, 2})
					auth := make([]byte, 2)
					io.ReadFull(r, auth)
					username := make([]byte, int(auth[1]))
					io.ReadFull(r, username)
					pn, _ := r.ReadByte()
					password := make([]byte, int(pn))
					io.ReadFull(r, password)
					if string(username) != "user" || string(password) != "pass" {
						fail(fmt.Errorf("wrong SOCKS5 credentials"))
						return
					}
					conn.Write([]byte{1, 0})
					head = make([]byte, 4)
					io.ReadFull(r, head)
					if head[3] != 3 {
						fail(fmt.Errorf("destination resolved locally"))
						return
					}
					size, _ := r.ReadByte()
					domain := make([]byte, int(size))
					io.ReadFull(r, domain)
					port := make([]byte, 2)
					io.ReadFull(r, port)
					if string(domain) != "unresolvable.invalid" || port[0] != 3 || port[1] != 225 {
						fail(fmt.Errorf("wrong destination"))
						return
					}
					conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 1})
				} else {
					head := make([]byte, 8)
					if _, err = io.ReadFull(r, head); err != nil {
						fail(err)
						return
					}
					username, _ := r.ReadString(0)
					domain, _ := r.ReadString(0)
					if head[0] != 4 || head[7] != 1 || username != "user\x00" || domain != "unresolvable.invalid\x00" {
						fail(fmt.Errorf("wrong SOCKS4a destination or auth"))
						return
					}
					conn.Write([]byte{0, 90, 0, 0, 0, 0, 0, 0})
				}
				io.WriteString(conn, "ok")
				result <- nil
			}()
			host, port, _ := net.SplitHostPort(listener.Addr().String())
			pn, _ := strconv.Atoi(port)
			n, err := NewNetwork(&Proxy{Enabled: true, Scheme: scheme, Host: host, Port: pn, Username: "user", Password: "pass"}, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer n.Close()
			conn, err := n.dial(context.Background(), "tcp", "unresolvable.invalid:993")
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			b := make([]byte, 2)
			if _, err = io.ReadFull(conn, b); err != nil {
				t.Fatal(err)
			}
			if string(b) != "ok" {
				t.Fatal("proxy payload corrupted")
			}
			if err = <-result; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStalledProxyHandshakeHasDeadline(t *testing.T) {
	for _, scheme := range []string{"socks4", "socks5", "http"} {
		t.Run(scheme, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan struct{})
			defer close(done)
			go func() {
				conn, err := listener.Accept()
				if err == nil {
					defer conn.Close()
					<-done
				}
			}()
			host, port, _ := net.SplitHostPort(listener.Addr().String())
			pn, _ := strconv.Atoi(port)
			n, err := NewNetwork(&Proxy{Enabled: true, Scheme: scheme, Host: host, Port: pn}, 50*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			defer n.Close()
			started := time.Now()
			conn, err := n.dial(context.Background(), "tcp", "unresolvable.invalid:993")
			if conn != nil {
				conn.Close()
			}
			if err == nil || time.Since(started) > time.Second {
				t.Fatalf("proxy handshake was not bounded: %v", err)
			}
		})
	}
}

func TestHTTPAPIUsesProxyOnceWithAbsoluteRequestAndAuthentication(t *testing.T) {
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == "CONNECT" {
			t.Error("ordinary HTTP API should not be double-tunnelled")
		}
		if r.URL.Host != "unresolvable.invalid" || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("wrong proxy request: %s", r.RequestURI)
		}
		if r.Header.Get("Proxy-Authorization") == "" {
			t.Error("proxy credentials were omitted")
		}
		io.WriteString(w, `{"ok":true}`)
	}))
	defer proxy.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(proxy.URL, "http://"))
	pn, _ := strconv.Atoi(port)
	n, err := NewNetwork(&Proxy{Enabled: true, Scheme: "http", Host: host, Port: pn, Username: "user", Password: "pass"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	resp, err := n.Client(time.Second).Post("http://unresolvable.invalid/v1/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if requests.Load() != 1 {
		t.Fatal("API request crossed proxy more than once")
	}
}

type noisyBackend struct{ backend.Backend }

func (b noisyBackend) Login(info *imap.ConnInfo, username, password string) (backend.User, error) {
	u, err := b.Backend.Login(info, username, password)
	return noisyUser{u}, err
}

type noisyUser struct{ backend.User }

func (u noisyUser) GetMailbox(name string) (backend.Mailbox, error) {
	m, err := u.User.GetMailbox(name)
	return noisyMailbox{m}, err
}

type noisyMailbox struct{ backend.Mailbox }

func (m noisyMailbox) ListMessages(uid bool, set *imap.SeqSet, items []imap.FetchItem, out chan<- *imap.Message) error {
	defer close(out)
	in := make(chan *imap.Message)
	done := make(chan error, 1)
	go func() { done <- m.Mailbox.ListMessages(uid, set, items, in) }()
	for msg := range in {
		if len(msg.Body) == 0 {
			for range 4 {
				out <- msg
			}
		} else {
			// Body literals are streams and can only be serialized once.
			out <- msg
		}
		if len(msg.Body) > 0 {
			update := imap.NewMessage(msg.SeqNum, []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size})
			update.Uid = msg.Uid
			update.Size = 123
			out <- update
		}
	}
	return <-done
}

func TestFetchDrainsMultipleResponsesForOneUID(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(noisyBackend{memory.New()})
	srv.AllowInsecureAuth = true
	go srv.Serve(listener)
	defer srv.Close()
	c, err := client.Dial(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Terminate()
	c.Timeout = time.Second
	if err = c.Login("username", "password"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Select("INBOX", true); err != nil {
		t.Fatal(err)
	}
	msg, err := fetchOne(c, 6, []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size})
	if err != nil || msg == nil || msg.Size == 0 {
		t.Fatalf("FETCH did not drain to completion: %+v %v", msg, err)
	}
	section := &imap.BodySectionName{Peek: true}
	msg, err = fetchOne(c, 6, []imap.FetchItem{imap.FetchUid, section.FetchItem()})
	if err != nil || msg == nil || msg.GetBody(section) == nil {
		t.Fatalf("later FETCH update discarded message body: %+v %v", msg, err)
	}
	body, err := io.ReadAll(msg.GetBody(section))
	if err != nil || len(body) == 0 {
		t.Fatalf("message body unavailable after FETCH completion: %q %v", body, err)
	}
}
