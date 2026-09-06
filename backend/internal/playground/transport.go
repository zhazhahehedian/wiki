package playground

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

func NormalizeURL(raw string) (string, error) {
	u, e := url.Parse(strings.TrimSpace(raw))
	if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || len(raw) > 2048 {
		return "", ErrInvalid
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(u.Path, "/chat/completions") {
		return "", ErrInvalid
	}
	return strings.TrimRight(u.String(), "/"), nil
}
func publicIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96"} {
		if netip.MustParsePrefix(s).Contains(a) {
			return false
		}
	}
	return true
}

type guardedTransport struct {
	public, trusted *http.Transport
	origins         map[string]bool
}

func (t *guardedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	origin := r.URL.Scheme + "://" + r.URL.Host
	if t.origins[origin] {
		return t.trusted.RoundTrip(r)
	}
	if r.URL.Scheme != "https" {
		return nil, ErrInvalid
	}
	return t.public.RoundTrip(r)
}

// Public destinations are resolved, checked and dialed by IP (DNS rebinding
// protection). Only explicit operator-trusted origins can use private addresses
// or the system proxy. Redirects never receive a user's Authorization header.
func NewHTTPClient(allowedOrigins string) (*http.Client, error) {
	origins := map[string]bool{}
	for _, raw := range strings.Split(allowedOrigins, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("invalid playground trusted origin")
		}
		origins[raw] = true
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ResponseHeaderTimeout: 15 * time.Second, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 60 * time.Second, MaxIdleConns: 20}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(addr)
		if e != nil {
			return nil, ErrInvalid
		}
		ips, e := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if e != nil || len(ips) == 0 {
			return nil, ErrUpstream
		}
		for _, ip := range ips {
			if !publicIP(ip) {
				return nil, ErrInvalid
			}
		}
		var last error
		for _, ip := range ips {
			c, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return c, nil
			}
			last = e
		}
		return nil, last
	}
	trusted := transport.Clone()
	trusted.Proxy = http.ProxyFromEnvironment
	trusted.DialContext = dialer.DialContext
	return &http.Client{Transport: &guardedTransport{transport, trusted, origins}, Timeout: 50 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}
