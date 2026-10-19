package mailbot

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

type imageRoundTrip func(*http.Request) (*http.Response, error)

func (f imageRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestImageProxyBlocksNonPublicDestinations(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "100.100.100.200", "198.18.1.95", "192.168.1.1", "0.0.0.0", "224.0.0.1", "::1", "::ffff:127.0.0.1", "fd00::1", "fe80::1", "64:ff9b::7f00:1", "2002:7f00:1::", "2001:db8::1"} {
		if publicImageIP(netip.MustParseAddr(raw)) {
			t.Errorf("allowed %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicImageIP(netip.MustParseAddr(raw)) {
			t.Errorf("blocked public %s", raw)
		}
	}
	if conn, err := dialImage(context.Background(), "tcp", "127.0.0.1:80"); err == nil {
		conn.Close()
		t.Fatal("dialed loopback")
	}
	for _, raw := range []string{"file:///etc/passwd", "http://user:pass@example.com/a", "http://example.com:8080/a", "data:image/png,abc"} {
		if _, err := imageURL(raw); err == nil {
			t.Errorf("allowed %s", raw)
		}
	}
	client := newImageClient()
	if client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("environment proxy can bypass destination validation")
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1/a", nil)
	if client.CheckRedirect(r, make([]*http.Request, 3)) == nil {
		t.Fatal("redirect limit ignored")
	}
}

func TestImageProxyPinsIPBeforeConfiguredEgress(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "8.8.8.8"} {
		called := false
		_, err := dialCheckedImage(context.Background(), "tcp", host+":443", net.DefaultResolver, func(_ context.Context, _, address string) (net.Conn, error) {
			called = true
			if address != "8.8.8.8:443" {
				t.Fatalf("unchecked destination passed to proxy: %s", address)
			}
			return nil, io.EOF
		})
		if err == nil || called != (host == "8.8.8.8") {
			t.Fatal("destination validation was bypassed")
		}
	}
	client := newImageClient(&Proxy{Enabled: true, Scheme: "invalid"})
	if _, err := client.Get("https://example.com/a"); err == nil {
		t.Fatal("invalid proxy must fail closed")
	}
}

func TestRemoteImageValidationCacheAndAuthorization(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aWQAAAABJRU5ErkJggg==")
	count := 0
	client := &http.Client{Transport: imageRoundTrip(func(r *http.Request) (*http.Response, error) {
		count++
		if r.Header.Get("Cookie") != "" || r.Header.Get("Referer") != "" || r.Header.Get("User-Agent") != "MailBot-ImageProxy/1.0" {
			t.Fatal("viewer metadata leaked")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(png)), Header: make(http.Header)}, nil
	})}
	s := &Service{store: testStore(t)}
	raw := "https://images.example.com/logo.png"
	for range 2 {
		data, kind, err := s.store.remoteImage(context.Background(), client, raw)
		if err != nil || kind != "image/png" || !bytes.Equal(data, png) {
			t.Fatalf("image: %s %v", kind, err)
		}
	}
	if count != 1 {
		t.Fatal("cache did not avoid repeated fetch")
	}
	e := Email{ID: "remote", HTMLBody: `<img src="` + raw + `"><img srcset="https://tracker.invalid/a 2x">`}
	if err := s.store.Commit("a", "INBOX", 1, 1, []Email{e}, []string{"t"}); err != nil {
		t.Fatal(err)
	}
	token, _ := s.store.readingToken(e.ID)
	h := s.readerHandler()
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	path := "/read/" + token
	w := get(path + "/remote/" + imageKey(raw))
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), png) || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("authorized cached image unavailable")
	}
	for _, p := range []string{"/read/" + strings.Repeat("0", 64) + "/remote/" + imageKey(raw), path + "/remote/" + imageKey("https://other.invalid/")} {
		if get(p).Code != 404 {
			t.Fatal("proxy allowed unauthorized source")
		}
	}
	w = get(path + "/body")
	if !strings.Contains(w.Body.String(), "/remote/") || strings.Contains(w.Body.String(), "srcset") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "img-src 'self';") {
		t.Fatal("default images bypassed proxy or missing")
	}
	if !strings.Contains(get(path+"/body?images=0").Body.String(), "/remote/") {
		t.Fatal("legacy image query must not disable images")
	}
	if strings.Contains(get(path).Body.String(), "隐藏图片") || strings.Contains(get(path).Body.String(), "显示图片") {
		t.Fatal("image toggle must not appear")
	}
}

func TestImageProxyRejectsActiveAndOversizedResponses(t *testing.T) {
	for _, body := range []string{`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`, `<html>not an image</html>`, strings.Repeat("x", maxRemoteImage+1)} {
		client := &http.Client{Transport: imageRoundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}
		if _, _, err := fetchRemoteImage(context.Background(), client, "https://images.example.com/a"); err == nil {
			t.Fatal("accepted unsafe response")
		}
	}
}
