package mailbot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
	xhtml "golang.org/x/net/html"
)

const maxRemoteImage = 5 << 20

func imageURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return nil, fmt.Errorf("invalid image URL")
	}
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		return nil, fmt.Errorf("unsupported image port")
	}
	return u, nil
}

func imageKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Only image sources actually present in this mail may be fetched. A URL query
// parameter would turn the private reader into an arbitrary forwarding proxy.
func remoteImageSource(e Email, key string) string {
	doc, err := xhtml.Parse(strings.NewReader(e.HTMLBody))
	if err != nil {
		return ""
	}
	var found string
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "img" {
			for _, a := range n.Attr {
				if a.Key == "src" && imageKey(a.Val) == key {
					if _, err := imageURL(a.Val); err == nil {
						found = a.Val
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

var blockedImageNetworks = func() []netip.Prefix {
	var out []netip.Prefix
	for _, cidr := range strings.Fields(`0.0.0.0/8 10.0.0.0/8 100.64.0.0/10 127.0.0.0/8 169.254.0.0/16 172.16.0.0/12 192.0.0.0/24 192.0.2.0/24 192.88.99.0/24 192.168.0.0/16 198.18.0.0/15 198.51.100.0/24 203.0.113.0/24 224.0.0.0/3 ::/96 ::ffff:0:0/96 64:ff9b::/96 64:ff9b:1::/48 100::/64 2001::/23 2001:db8::/32 2002::/16 fc00::/7 fe80::/10 ff00::/8`) {
		out = append(out, netip.MustParsePrefix(cidr))
	}
	return out
}()

func publicImageIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.Zone() != "" {
		return false
	}
	for _, prefix := range blockedImageNetworks {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

// Resolve and validate at connection time, then dial the checked IP directly:
// a second DNS lookup must never allow rebinding to an internal address.
func dialImage(ctx context.Context, network, address string) (net.Conn, error) {
	return dialCheckedImage(ctx, network, address, net.DefaultResolver, (&net.Dialer{Timeout: 4 * time.Second}).DialContext)
}

func dialCheckedImage(ctx context.Context, network, address string, resolver *net.Resolver, dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("image DNS unavailable")
	}
	local, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if !publicImageIP(ip) {
			return nil, fmt.Errorf("image address blocked")
		}
		for _, a := range local {
			if p, err := netip.ParsePrefix(a.String()); err == nil && p.Addr().Unmap() == ip.Unmap() {
				return nil, fmt.Errorf("local image address blocked")
			}
		}
	}
	var last error
	for _, ip := range ips {
		conn, err := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

func newImageClient(proxies ...*Proxy) *http.Client {
	dial := dialImage
	if len(proxies) > 0 && proxies[0] != nil && proxies[0].Enabled {
		n, err := NewNetwork(proxies[0], 4*time.Second)
		if err != nil {
			dial = func(context.Context, string, string) (net.Conn, error) { return nil, err }
		} else {
			// Use the configured egress for DNS too. The proxy sees only checked
			// destination IPs, never a hostname it could resolve to a private IP.
			resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return n.dial(ctx, "tcp", "1.1.1.1:53")
			}}
			dial = func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialCheckedImage(ctx, network, address, resolver, n.dial)
			}
		}
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{DialContext: dial, DisableKeepAlives: true, TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 32 << 10},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many image redirects")
			}
			_, err := imageURL(req.URL.String())
			return err
		},
	}
}

func fetchRemoteImage(ctx context.Context, client *http.Client, raw string) ([]byte, string, error) {
	if _, err := imageURL(raw); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "MailBot-ImageProxy/1.0")
	req.Header.Set("Accept", "image/png,image/jpeg,image/gif,image/webp")
	res, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.ContentLength > maxRemoteImage {
		return nil, "", fmt.Errorf("image unavailable or too large")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxRemoteImage+1))
	if err != nil || len(data) > maxRemoteImage {
		return nil, "", fmt.Errorf("image download failed")
	}
	kind := http.DetectContentType(data)
	if !safeImageType(kind) {
		return nil, "", fmt.Errorf("unsupported image type")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return nil, "", fmt.Errorf("invalid image dimensions")
	}
	return data, kind, nil
}

func (s *Store) remoteImage(ctx context.Context, client *http.Client, raw string) ([]byte, string, error) {
	key := imageKey(raw)
	var data []byte
	var kind string
	if err := s.db.QueryRowContext(ctx, `SELECT data,type FROM image_cache WHERE key=? AND created>?`, key, time.Now().Add(-30*24*time.Hour).Unix()).Scan(&data, &kind); err == nil {
		return data, kind, nil
	}
	data, kind, err := fetchRemoteImage(ctx, client, raw)
	if err != nil {
		return nil, "", err
	}
	// Bound persistent storage as well as individual downloads. Cache insertion
	// and eviction share a transaction so concurrent requests cannot exceed it.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM image_cache WHERE created<=?`, time.Now().Add(-30*24*time.Hour).Unix()); err != nil {
		return nil, "", err
	}
	for {
		var size int
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(length(data)),0) FROM image_cache`).Scan(&size); err != nil {
			return nil, "", err
		}
		if size+len(data) <= 64<<20 {
			break
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM image_cache WHERE key=(SELECT key FROM image_cache ORDER BY created,key LIMIT 1)`); err != nil {
			return nil, "", err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO image_cache(key,data,type,created) VALUES(?,?,?,?)`, key, data, kind, time.Now().Unix()); err != nil {
		return nil, "", err
	}
	if err = tx.Commit(); err != nil {
		return nil, "", err
	}
	return data, kind, nil
}
