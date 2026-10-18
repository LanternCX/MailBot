package mailbot

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMailHTMLPreservesEmailTypographyAndSpacing(t *testing.T) {
	e := Email{HTMLBody: `<table><tr><td style="color: #272727; font-family: 'Open Sans', Arial, Geneva, sans-serif; font-size: 15px; mso-line-height-rule: exactly; line-height: 20px; font-weight: 600;">Hi user,</td></tr><tr><td style="padding-top: 16px; line-height: 0px;"></td></tr><tr><td style="border-bottom: 1px solid #A3DFF9; padding-bottom: 18px;">Footer</td></tr></table>`}
	clean := renderMailHTML(e, strings.Repeat("a", 64), false)
	for _, want := range []string{"font-family:", "font-size: 15px", "font-weight: 600", "line-height: 20px", "padding-top: 16px", "border-bottom: 1px solid #A3DFF9"} {
		if !strings.Contains(clean, want) {
			t.Errorf("missing %q in %s", want, clean)
		}
	}
}

func TestMailHTMLPreservesStylesWithoutActiveContent(t *testing.T) {
	e := Email{HTMLBody: `<html><head><style>@import url(https://tracker.invalid/style);body{background-color:#fafafa}.receipt{color:red;padding:20px;width:600px;background-image:url(https://tracker.invalid/pixel)}@media screen and (max-width:600px){.receipt{width:100%}}</style></head><body class="receipt" style="font-family:Georgia"><table cellpadding="12"><tr><td style="background-color:#eeeeee">Original layout</td></tr></table><img src="cid:logo"><img src="https://images.example.com/banner.png"><script>alert(1)</script><form><input></form><a href="javascript:alert(1)" onclick="alert(1)">link</a><style>.bad{color:red}</style></body></html>`, Images: []InlineImage{{CID: "logo", Type: "image/png", Data: []byte{1}}}}
	clean := renderMailHTML(e, strings.Repeat("a", 64), false)
	for _, want := range []string{".receipt{", "color: red", "@media screen", "width: 100%", "cellpadding=\"12\"", "class=\"receipt\"", "font-family: Georgia", "Original layout", "/image/0"} {
		if !strings.Contains(clean, want) {
			t.Fatalf("missing %q: %s", want, clean)
		}
	}
	for _, bad := range []string{"tracker.invalid", "images.example.com", "<script", "<form", "<input", "onclick", "javascript:"} {
		if strings.Contains(clean, bad) {
			t.Fatalf("unsafe/unapproved content %q: %s", bad, clean)
		}
	}
	if clean := renderMailHTML(e, strings.Repeat("a", 64), true); !strings.Contains(clean, "/remote/"+imageKey("https://images.example.com/banner.png")) || strings.Contains(clean, "https://images.example.com/banner.png") {
		t.Fatal("external image must use private proxy")
	}
}

func TestInlineMIMEImagesSurviveStorageAndRequireMailLink(t *testing.T) {
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aWQAAAABJRU5ErkJggg=="
	raw := "Content-Type: multipart/related; boundary=x\r\n\r\n--x\r\nContent-Type: text/html\r\n\r\n<p>Inline logo</p><img src=\"cid:logo\">\r\n--x\r\nContent-Type: image/png; name=logo.png\r\nContent-ID: <logo>\r\nContent-Disposition: inline; filename=logo.png\r\nContent-Transfer-Encoding: base64\r\n\r\n" + png + "\r\n--x\r\nContent-Type: image/svg+xml\r\nContent-ID: <unsafe>\r\n\r\n<svg onload=\"alert(1)\"></svg>\r\n--x--\r\n"
	e := Email{ID: "images"}
	if err := parseEmail(strings.NewReader(raw), &e); err != nil {
		t.Fatal(err)
	}
	if len(e.Images) != 1 || e.Images[0].CID != "logo" {
		t.Fatalf("images: %+v", e.Images)
	}
	s := &Service{store: testStore(t)}
	if err := s.store.Commit("a", "INBOX", 1, 1, []Email{e}, []string{"t"}); err != nil {
		t.Fatal(err)
	}
	token, err := s.store.readingToken(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.readerHandler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	w := get("/read/" + token + "/image/0")
	want, _ := base64.StdEncoding.DecodeString(png)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Body.String() != string(want) {
		t.Fatal("inline image unavailable")
	}
	for _, path := range []string{"/read/" + strings.Repeat("0", 64) + "/image/0", "/read/" + token + "/image/-1", "/read/" + token + "/image/1"} {
		if get(path).Code != 404 {
			t.Fatal("image authorization failed")
		}
	}
	w = get("/read/" + token + "/body")
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "img-src 'self';") || !strings.Contains(w.Body.String(), "/image/0") {
		t.Fatal("default image policy failed")
	}
	w = get("/read/" + token + "/body?images=1")
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "img-src 'self';") {
		t.Fatal("external image must not bypass proxy")
	}
}
