package mailbot

import (
	"strings"
	"testing"
)

func TestPlainMailPreservesLegacyWhitespaceAndPartJoining(t *testing.T) {
	for _, tc := range []struct{ contentType, body, want string }{
		{"text/plain; charset=utf-8", "  Keep spaces\r\n\r\n", "  Keep spaces\r\n\r\n"},
		{"multipart/mixed; boundary=x", "--x\r\nContent-Type: text/plain\r\n\r\nfirst\r\n--x\r\nContent-Type: text/plain\r\n\r\nsecond\r\n--x--\r\n", "firstsecond"},
		{"multipart/alternative; boundary=x", "--x\r\nContent-Type: text/plain\r\n\r\n  \r\n--x\r\nContent-Type: text/html\r\n\r\n<b>must not replace whitespace plain text</b>\r\n--x--\r\n", "  "},
	} {
		var e Email
		raw := "From: sender@example.org\r\nContent-Type: " + tc.contentType + "\r\n\r\n" + tc.body
		if err := parseEmail(strings.NewReader(raw), &e); err != nil || e.Body != tc.want {
			t.Fatalf("body %q, want %q: %v", e.Body, tc.want, err)
		}
	}
}

func TestMailBodyExcludesNamedInlineTextAttachments(t *testing.T) {
	for _, attachmentHeaders := range []string{
		"Content-Type: text/plain; name=notes.txt\r\nContent-Disposition: inline",
		"Content-Type: text/plain\r\nContent-Disposition: inline; filename=notes.txt",
		"Content-Type: text/html; name=notes.html",
	} {
		raw := "Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\nmail body\r\n--x\r\n" + attachmentHeaders + "\r\n\r\nPRIVATE ATTACHMENT\r\n--x--\r\n"
		var e Email
		if err := parseEmail(strings.NewReader(raw), &e); err != nil || e.Body != "mail body" {
			t.Fatalf("attachment included in body %q: %v", e.Body, err)
		}
	}
}
