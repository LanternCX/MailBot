package mailbot

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	gomessage "github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"golang.org/x/net/html"
)

func (s *Service) connectIMAP(ctx context.Context, a Account) (*client.Client, net.Conn, func(), error) {
	conn, err := s.network.dial(ctx, "tcp", net.JoinHostPort(a.Host, fmt.Sprint(a.Port)))
	if err != nil {
		return nil, nil, nil, err
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { rawConn.Close() })
	cleanup := func() { stop(); rawConn.Close() }
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()
	conn.SetDeadline(time.Now().Add(s.timeout))
	if a.SSL {
		tc := tls.Client(conn, &tls.Config{ServerName: a.Host, MinVersion: tls.VersionTLS12})
		if err = tc.HandshakeContext(ctx); err != nil {
			return nil, nil, nil, err
		}
		conn = tc
	}
	c, err := client.New(conn)
	if err != nil {
		return nil, nil, nil, err
	}
	c.Timeout = s.timeout
	conn.SetDeadline(time.Time{})
	if err = c.Login(a.Email, a.Password); err != nil {
		return nil, nil, nil, fmt.Errorf("IMAP login failed")
	}
	success = true
	return c, conn, func() { c.Terminate(); cleanup() }, nil
}

func (s *Service) pollAccount(ctx context.Context, a Account) error {
	c, _, closeClient, err := s.connectIMAP(ctx, a)
	if err != nil {
		return err
	}
	defer closeClient()
	var folderErrors []error
	for _, folder := range a.Folders {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err = s.pollFolder(c, a, folder); err != nil {
			folderErrors = append(folderErrors, fmt.Errorf("folder %q: %w", folder, err))
			if c.State() == imap.LogoutState {
				break
			}
		}
	}
	return errors.Join(folderErrors...)
}

func mailboxStatusKey(a Account) string {
	if len(a.Folders) == 1 && a.Folders[0] != "INBOX" {
		return a.Name + "/" + a.Folders[0]
	}
	return a.Name
}

// One connection per folder keeps all configured folders live at once.
func (s *Service) watchFolder(ctx context.Context, a Account) error {
	c, conn, closeClient, err := s.connectIMAP(ctx, a)
	if err != nil {
		return err
	}
	idle, err := c.Support("IDLE")
	if err != nil {
		closeClient()
		return err
	}
	updates := make(chan client.Update, 64)
	c.Updates = updates
	notify := make(chan struct{}, 1)
	drainCtx, cancelDrain := context.WithCancel(context.Background())
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			select {
			case <-drainCtx.Done():
				return
			case update := <-updates:
				if _, ok := update.(*client.MailboxUpdate); ok {
					select {
					case notify <- struct{}{}:
					default:
					}
				}
			}
		}
	}()
	defer func() { closeClient(); cancelDrain(); <-drained }()
	folder := a.Folders[0]
	slog.Info("mailbox watcher connected", "account", a.Name, "folder", folder, "idle", idle)
	if _, err = c.Select(folder, !s.config.MarkSeen); err != nil {
		return err
	}
	for ctx.Err() == nil {
		// Coalesce updates before scanning, retaining any notification arriving
		// during a scan so a message cannot fall into the scan/IDLE gap.
		select {
		case <-notify:
		default:
		}
		if err = s.pollSelectedFolder(c, a, folder, c.Mailbox()); err != nil {
			return err
		}
		s.mu.Lock()
		s.status[mailboxStatusKey(a)] = "ok"
		s.lastPoll[mailboxStatusKey(a)] = time.Now()
		s.mu.Unlock()
		if idle {
			interval := min(time.Minute, time.Duration(s.config.PollInterval)*time.Second)
			if err = s.idleMailbox(ctx, c, notify, interval); err != nil {
				return err
			}
		} else {
			// go-imap leaves the last command deadline on the socket. Remove it
			// while waiting; the next command installs its own bounded deadline.
			if err = conn.SetDeadline(time.Time{}); err != nil {
				return err
			}
			if !wait(ctx, time.Duration(s.config.PollInterval)*time.Second) {
				return ctx.Err()
			}
		}
	}
	return ctx.Err()
}

// Periodic scans recover missed notifications and renew IDLE well before the
// server's inactivity limit. DONE is bounded even when a server stops replying.
func (s *Service) idleMailbox(ctx context.Context, c *client.Client, notify <-chan struct{}, interval time.Duration) error {
	c.Timeout = interval + s.timeout
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- c.Idle(stop, &client.IdleOptions{LogoutTimeout: -1, PollInterval: -1}) }()
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case err := <-done:
		close(stop)
		c.Timeout = s.timeout
		return err
	case <-ctx.Done():
	case <-notify:
	case <-timer.C:
	}
	close(stop)
	deadline := time.NewTimer(s.timeout)
	defer deadline.Stop()
	select {
	case err := <-done:
		c.Timeout = s.timeout
		return err
	case <-deadline.C:
		c.Terminate()
		<-done
		c.Timeout = s.timeout
		return fmt.Errorf("IMAP IDLE did not stop before deadline")
	}
}

func (s *Service) pollFolder(c *client.Client, a Account, folder string) error {
	mb, err := c.Select(folder, !s.config.MarkSeen)
	if err != nil {
		return err
	}
	return s.pollSelectedFolder(c, a, folder, mb)
}

func (s *Service) pollSelectedFolder(c *client.Client, a Account, folder string, mb *imap.MailboxStatus) error {
	account := accountID(a)
	validity, uid, exists, err := s.store.Cursor(account, folder)
	if err != nil {
		return err
	}
	if !exists || validity != mb.UidValidity {
		last := uint32(0)
		if mb.UidNext > 0 {
			last = mb.UidNext - 1
		}
		slog.Info("mailbox baseline saved", "account", a.Name, "folder", folder, "uid", last, "uidvalidity", mb.UidValidity)
		return s.store.Baseline(account, folder, mb.UidValidity, last)
	}
	if uid == ^uint32(0) {
		return nil
	}
	criteria := imap.NewSearchCriteria()
	criteria.Uid = new(imap.SeqSet)
	criteria.Uid.AddRange(uid+1, 0)
	if s.config.UnreadOnly {
		criteria.WithoutFlags = []string{imap.SeenFlag}
	}
	uids, err := c.UidSearch(criteria)
	if err != nil {
		return err
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	// Fetch individually to bound memory and commit each successful message.
	for _, next := range uids {
		if next <= uid {
			continue
		}
		set := new(imap.SeqSet)
		set.AddNum(next)
		section := &imap.BodySectionName{Peek: true}
		msg, err := fetchOne(c, next, []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size})
		if err != nil {
			return err
		}
		if msg == nil {
			continue
		}
		var e Email
		e = Email{ID: emailID(account, folder, validity, next), Account: a.Name, Folder: folder, UID: next}
		limit := s.config.MaxMessageBytes
		if limit > 0 && int64(msg.Size) > limit {
			e.Subject = "[Large email]"
			e.Body = fmt.Sprintf("Email is %d bytes; body exceeds the configured %d byte processing limit. Open your mailbox to read it.", msg.Size, limit)
		} else {
			msg, err = fetchOne(c, next, []imap.FetchItem{imap.FetchUid, section.FetchItem()})
			if err != nil {
				return err
			}
			if msg == nil {
				continue
			}
			body := msg.GetBody(section)
			if body == nil {
				return fmt.Errorf("server omitted message body for UID %d", next)
			}
			var reader io.Reader = body
			if limit > 0 {
				reader = io.LimitReader(body, limit+1)
			}
			if err = parseEmail(reader, &e); err != nil {
				// A malformed email must not permanently block the mailbox.
				e.Subject = "[Unreadable email]"
				e.Body = "Unable to parse this email. Open your mailbox to read it."
				slog.Warn("email parse failed", "account", a.Name, "uid", next)
			}
		}
		e.WebLink = webLink(a, next)
		if err = s.store.Commit(account, folder, validity, next, []Email{e}, s.targets); err != nil {
			return err
		}
		uid = next
		if s.config.MarkSeen {
			if err = c.UidStore(set, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.SeenFlag}, nil); err != nil {
				return fmt.Errorf("mark queued email seen: %w", err)
			}
		}
		slog.Info("email queued", "account", a.Name, "folder", folder, "uid", next)
	}
	return nil
}

// Drain FETCH responses concurrently: servers may interleave multiple updates
// for the same UID before the command's tagged completion response.
func fetchOne(c *client.Client, uid uint32, items []imap.FetchItem) (*imap.Message, error) {
	set := new(imap.SeqSet)
	set.AddNum(uid)
	messages := make(chan *imap.Message)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(set, items, messages) }()
	var result *imap.Message
	for msg := range messages {
		if msg.Uid != uid {
			continue
		}
		if result == nil {
			result = msg
			continue
		}
		// Retain the requested size and body across separate FETCH updates.
		if _, ok := msg.Items[imap.FetchRFC822Size]; ok {
			result.Size = msg.Size
		}
		for section, body := range msg.Body {
			if result.Body == nil {
				result.Body = make(map[*imap.BodySectionName]imap.Literal)
			}
			result.Body[section] = body
		}
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return result, nil
}

func parseEmail(r io.Reader, e *Email) error {
	mr, err := mail.CreateReader(r)
	if err != nil && !(gomessage.IsUnknownCharset(err) && mr != nil) {
		return err
	}
	e.Subject, _ = mr.Header.Subject()
	if e.Subject == "" {
		e.Subject = "(No subject)"
	}
	e.Sender = mr.Header.Get("From")
	if addresses, err := mr.Header.AddressList("From"); err == nil && len(addresses) > 0 {
		e.Sender = addresses[0].Address
	}
	e.Date, _ = mr.Header.Date()
	var plain, rich strings.Builder
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil && !(gomessage.IsUnknownCharset(err) && part != nil) {
			return err
		}
		var h *gomessage.Header
		switch header := part.Header.(type) {
		case *mail.InlineHeader:
			h = &header.Header
		case *mail.AttachmentHeader:
			h = &header.Header
		default:
			continue
		}
		kind, params, _ := h.ContentType()
		_, disposition, _ := h.ContentDisposition()
		if params["name"] != "" || disposition["filename"] != "" {
			continue
		}
		if kind != "text/plain" && kind != "text/html" {
			continue
		}
		data, err := io.ReadAll(part.Body)
		if err != nil {
			return err
		}
		if kind == "text/plain" {
			plain.WriteString(strings.ToValidUTF8(string(data), "�"))
		} else {
			rich.WriteString(strings.ToValidUTF8(string(data), "�"))
		}
	}
	e.Body = plain.String()
	e.HTMLBody = rich.String()
	if e.Body == "" {
		e.Body = htmlText(rich.String())
	}
	return nil
}

var htmlSpaces = regexp.MustCompile(`[ \t]+`)
var htmlBlankLines = regexp.MustCompile(`\n\s*\n`)

func htmlText(body string) string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return ""
	}
	var out strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		block := false
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "head", "meta", "link":
				return
			case "br":
				out.WriteByte('\n')
			case "p", "div", "tr", "li":
				block = true
				out.WriteByte('\n')
			}
		}
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
			out.WriteByte(' ')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if block {
			out.WriteByte('\n')
		}
	}
	walk(doc)
	text := htmlSpaces.ReplaceAllString(out.String(), " ")
	return strings.TrimSpace(htmlBlankLines.ReplaceAllString(text, "\n\n"))
}

func webLink(a Account, uid uint32) string {
	if a.WebURL != "" {
		return strings.TrimRight(a.WebURL, "/") + "/" + fmt.Sprint(uid)
	}
	host := strings.ToLower(a.Host)
	switch {
	case strings.Contains(host, "gmail") || strings.Contains(host, "google"):
		return "https://mail.google.com/mail/u/0/#inbox/" + fmt.Sprint(uid)
	case strings.Contains(host, "outlook") || strings.Contains(host, "hotmail"):
		return "https://outlook.live.com/mail/0/inbox/id/" + fmt.Sprint(uid)
	case strings.Contains(host, "qq.com"):
		return "https://mail.qq.com/"
	case strings.Contains(host, "163.com") || strings.Contains(host, "126.com"):
		return "https://mail.163.com/"
	}
	return ""
}

func CheckIMAP(ctx context.Context, c Config, a Account) error {
	n, err := NewNetwork(c.Proxy, time.Duration(c.NetworkTimeout)*time.Second)
	if err != nil {
		return err
	}
	defer n.Close()
	s := &Service{network: n, timeout: time.Duration(c.NetworkTimeout) * time.Second}
	a.Folders = nil
	return s.pollAccount(ctx, a)
}
