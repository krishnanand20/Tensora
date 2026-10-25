package auth

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Transport decides what a request's connection can be trusted with: who the client is (for
// slowing down guessing), and whether passwords and session cookies may cross it.
type Transport struct {
	// TrustedProxies are reverse proxies whose X-Forwarded-For and X-Forwarded-Proto are believed
	// (TENSORA_TRUSTED_PROXIES). Anyone else's are ignored.
	TrustedProxies []netip.Prefix
	// AllowPlainHTTP allows sign-in and session cookies over plain HTTP from other machines
	// (TENSORA_INSECURE_HTTP=true), for a network the operator trusts.
	AllowPlainHTTP bool
}

func (t Transport) peer(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	return a.Unmap(), err == nil
}

func (t Transport) trusted(a netip.Addr) bool {
	for _, p := range t.TrustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// forwarded reports whether r came through a trusted proxy.
func (t Transport) forwarded(r *http.Request) bool {
	a, ok := t.peer(r)
	return ok && t.trusted(a)
}

// Client is the caller's address, for rate limiting. Behind a trusted proxy it's the nearest
// address in X-Forwarded-For that isn't a trusted proxy. An IPv6 address counts as its /64, which
// one person usually controls entirely.
func (t Transport) Client(r *http.Request) string {
	a, ok := t.peer(r)
	if !ok {
		return r.RemoteAddr
	}
	if t.trusted(a) {
		hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			h, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
			if err != nil {
				break
			}
			if h = h.Unmap(); !t.trusted(h) {
				a = h
				break
			}
		}
	}
	if a.Is6() {
		p, _ := a.Prefix(64)
		return p.String()
	}
	return a.String()
}

// Secure reports whether r arrived over HTTPS: directly, or through a trusted proxy that says so.
func (t Transport) Secure(r *http.Request) bool {
	return r.TLS != nil || (t.forwarded(r) && r.Header.Get("X-Forwarded-Proto") == "https")
}

// Private reports whether passwords and session cookies may cross r's connection: HTTPS, this
// machine (a loopback client that didn't come through a proxy), or the operator's opt-in.
func (t Transport) Private(r *http.Request) bool {
	if t.Secure(r) || t.AllowPlainHTTP {
		return true
	}
	a, ok := t.peer(r)
	return ok && a.IsLoopback() && !t.trusted(a)
}

// SessionCookieName is the cookie's name for r: over HTTPS, the __Host- form, which a browser only
// accepts with Secure, Path=/, and no Domain, so another site or subdomain can't plant or overwrite it.
func (t Transport) SessionCookieName(r *http.Request) string {
	if t.Secure(r) {
		return SecureSessionCookie
	}
	return SessionCookie
}
