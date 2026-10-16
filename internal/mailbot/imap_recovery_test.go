package mailbot

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-imap/server"
)

func TestMalformedMailDoesNotBlockFollowingMailOrRepeatAfterPoll(t *testing.T) {
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
	s := &Service{config: DefaultConfig(), store: testStore(t), targets: []string{"target"}}
	a := Account{Name: "test", Email: "username", Host: "localhost", Port: 993}
	if err = s.pollFolder(c, a, "INBOX"); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		"From: sender@example.org\r\nSubject: Broken MIME\r\nContent-Type: multipart/mixed\r\n\r\nmissing boundary",
		"From: sender@example.org\r\nSubject: Following mail\r\nContent-Type: text/plain\r\n\r\nkeep this body",
	} {
		if err = c.Append("INBOX", nil, time.Now(), strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.pollFolder(c, a, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if count, err := s.store.Pending(); err != nil || count != 2 {
		t.Fatalf("queue count %d: %v", count, err)
	}
	first, err := s.store.Next("target")
	if err != nil || first == nil || first.Email.Subject != "[Unreadable email]" || !strings.Contains(first.Email.Body, "Open your mailbox") {
		t.Fatalf("missing malformed-mail notice: %+v %v", first, err)
	}
	if err = s.store.Done(first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.store.Next("target")
	if err != nil || second == nil || second.Email.Subject != "Following mail" || second.Email.Body != "keep this body" {
		t.Fatalf("following mail lost: %+v %v", second, err)
	}
	if err = s.store.Done(second.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.pollFolder(c, a, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if count, err := s.store.Pending(); err != nil || count != 0 {
		t.Fatalf("mail repeated: %d %v", count, err)
	}
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}
	uids, err := c.UidSearch(criteria)
	if err != nil || len(uids) != 0 {
		t.Fatalf("durably queued mail not marked seen: %v %v", uids, err)
	}
}

func TestQueueWriteFailureKeepsMailUnreadAndRecoverable(t *testing.T) {
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
	s := &Service{config: DefaultConfig(), store: testStore(t), targets: []string{"target"}}
	a := Account{Name: "test", Email: "username", Host: "localhost", Port: 993}
	if err = s.pollFolder(c, a, "INBOX"); err != nil {
		t.Fatal(err)
	}
	_, before, _, err := s.store.Cursor(accountID(a), "INBOX")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Append("INBOX", nil, time.Now(), strings.NewReader("Subject: Must recover\r\n\r\nrecoverable body")); err != nil {
		t.Fatal(err)
	}
	// Fail the durable enqueue after its email insert to exercise rollback.
	if _, err = s.store.db.Exec(`CREATE TRIGGER fixture_queue_failure BEFORE INSERT ON deliveries BEGIN SELECT RAISE(ABORT, 'fixture storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.pollFolder(c, a, "INBOX"); err == nil {
		t.Fatal("queue write failure ignored")
	}
	_, after, _, err := s.store.Cursor(accountID(a), "INBOX")
	if err != nil || after != before {
		t.Fatalf("cursor advanced on failed enqueue: %d -> %d, %v", before, after, err)
	}
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}
	uids, err := c.UidSearch(criteria)
	if err != nil || len(uids) != 1 {
		t.Fatalf("failed enqueue marked mail seen: %v %v", uids, err)
	}
	if _, err = s.store.db.Exec(`DROP TRIGGER fixture_queue_failure`); err != nil {
		t.Fatal(err)
	}
	if err = s.pollFolder(c, a, "INBOX"); err != nil {
		t.Fatal(err)
	}
	d, err := s.store.Next("target")
	if err != nil || d == nil || d.Email.Subject != "Must recover" || d.Email.Body != "recoverable body" {
		t.Fatalf("mail lost after storage recovery: %+v %v", d, err)
	}
	if count, err := s.store.Pending(); err != nil || count != 1 {
		t.Fatalf("unexpected recovered queue count %d: %v", count, err)
	}
	uids, err = c.UidSearch(criteria)
	if err != nil || len(uids) != 0 {
		t.Fatalf("recovered queued mail not marked seen: %v %v", uids, err)
	}
}
