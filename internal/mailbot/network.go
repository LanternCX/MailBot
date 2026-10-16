package mailbot

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/proxy"
)

type Network struct {
	dial      func(context.Context, string, string) (net.Conn, error)
	transport *http.Transport
}

func NewNetwork(p *Proxy, timeout time.Duration) (*Network, error) {
	d := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	n := &Network{dial: d.DialContext}
	if p != nil && p.Enabled {
		address := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
		switch p.Scheme {
		case "socks5":
			var auth *proxy.Auth
			if p.Username != "" {
				auth = &proxy.Auth{User: p.Username, Password: p.Password}
			}
			pd, err := proxy.SOCKS5("tcp", address, auth, d)
			if err != nil {
				return nil, err
			}
			n.dial = pd.(proxy.ContextDialer).DialContext
		case "http", "socks4":
			n.dial = func(ctx context.Context, network, target string) (net.Conn, error) {
				conn, err := d.DialContext(ctx, "tcp", address)
				if err != nil {
					return nil, err
				}
				stop := context.AfterFunc(ctx, func() { conn.Close() })
				defer stop()
				conn.SetDeadline(time.Now().Add(timeout))
				fail := func(err error) (net.Conn, error) { conn.Close(); return nil, err }
				if p.Scheme == "http" {
					req := &http.Request{Method: "CONNECT", URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
					if p.Username != "" {
						req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(p.Username+":"+p.Password)))
					}
					if err = req.Write(conn); err != nil {
						return fail(err)
					}
					reader := bufio.NewReader(conn)
					resp, err := http.ReadResponse(reader, req)
					if err != nil {
						return fail(err)
					}
					if resp.StatusCode != 200 {
						return fail(fmt.Errorf("proxy CONNECT status %d", resp.StatusCode))
					}
					conn.SetDeadline(time.Time{})
					return &bufferedConn{conn, reader}, nil
				}
				host, port, err := net.SplitHostPort(target)
				if err != nil {
					return fail(err)
				}
				pn, _ := strconv.Atoi(port)
				// SOCKS4a leaves destination DNS resolution to the proxy.
				packet := []byte{4, 1, byte(pn >> 8), byte(pn), 0, 0, 0, 1}
				packet = append(packet, []byte(p.Username)...)
				packet = append(packet, 0)
				packet = append(packet, []byte(host)...)
				packet = append(packet, 0)
				if _, err = conn.Write(packet); err != nil {
					return fail(err)
				}
				reply := make([]byte, 8)
				if _, err = io.ReadFull(conn, reply); err != nil {
					return fail(err)
				}
				if reply[1] != 90 {
					return fail(fmt.Errorf("SOCKS4a request rejected"))
				}
				conn.SetDeadline(time.Time{})
				return conn, nil
			}
		default:
			return nil, fmt.Errorf("unsupported proxy scheme %q", p.Scheme)
		}
	}
	dial := n.dial
	n.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return dial(ctx, network, address)
	}
	n.transport = &http.Transport{DialContext: n.dial, MaxIdleConns: 20, MaxIdleConnsPerHost: 4, IdleConnTimeout: 60 * time.Second, TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout}
	if p != nil && p.Enabled && p.Scheme == "http" {
		u := &url.URL{Scheme: "http", Host: net.JoinHostPort(p.Host, strconv.Itoa(p.Port))}
		if p.Username != "" {
			u.User = url.UserPassword(p.Username, p.Password)
		}
		// HTTP transport handles its own absolute-form requests and CONNECT.
		// Dial the proxy directly here; n.dial is reserved for IMAP tunnels.
		n.transport.Proxy = http.ProxyURL(u)
		n.transport.DialContext = d.DialContext
	}
	return n, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) { return c.reader.Read(b) }
func (n *Network) Client(timeout time.Duration) *http.Client {
	return &http.Client{Transport: n.transport, Timeout: timeout}
}
func (n *Network) Close() { n.transport.CloseIdleConnections() }
