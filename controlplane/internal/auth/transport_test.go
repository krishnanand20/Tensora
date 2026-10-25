package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func request(remote string, header http.Header) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	for k, vs := range header {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	return r
}

// Client is who's guessing, for the sign-in limits: a forwarded address is believed only from a
// trusted proxy, and only the entries the proxies themselves added.
func TestClient(t *testing.T) {
	tr := Transport{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")}}
	xff := func(lines ...string) http.Header { return http.Header{"X-Forwarded-For": lines} }
	tests := []struct {
		name, remote string
		header       http.Header
		want         string // "" means the client can't be told
	}{
		{"direct", "198.51.100.7:4000", nil, "198.51.100.7"},
		{"forwarded by an untrusted peer is ignored", "198.51.100.7:4000", xff("203.0.113.9"), "198.51.100.7"},
		{"through a trusted proxy", "10.0.0.5:4000", xff("203.0.113.9"), "203.0.113.9"},
		{"the nearest untrusted hop, not a made-up first one", "10.0.0.5:4000", xff("1.2.3.4, 203.0.113.9, 10.0.0.6"), "203.0.113.9"},
		{"every header line counts, not just the client's own", "10.0.0.5:4000", xff("6.6.6.6", "203.0.113.9"), "203.0.113.9"},
		{"ports are ignored", "10.0.0.5:4000", xff("203.0.113.9:5678"), "203.0.113.9"},
		{"IPv6 with a port", "10.0.0.5:4000", xff("[2001:db8::1]:5678"), "2001:db8::1"},
		{"an entry that can't be read means unknown, not the proxy", "10.0.0.5:4000", xff("203.0.113.9, unknown"), ""},
		{"a trusted proxy's own request", "10.0.0.5:4000", nil, "10.0.0.5"},
		{"proxied without X-Forwarded-For means unknown", "10.0.0.5:4000",
			http.Header{"X-Real-Ip": {"203.0.113.9"}, "X-Forwarded-Proto": {"https"}}, ""},
		{"only trusted proxies: the leftmost", "10.0.0.5:4000", xff("10.0.0.9, 10.0.0.6"), "10.0.0.9"},
		{"IPv4-mapped IPv6 is IPv4", "[::ffff:198.51.100.7]:4000", nil, "198.51.100.7"},
	}
	for _, tt := range tests {
		got, ok := tr.Client(request(tt.remote, tt.header))
		if (tt.want == "" && ok) || (tt.want != "" && (!ok || got.String() != tt.want)) {
			t.Errorf("%s: Client = %v, %v; want %q", tt.name, got, ok, tt.want)
		}
	}
}

// Passwords and session cookies cross only private connections, and never through a proxy the
// operator hasn't listed: everyone behind it would look like one caller, and what it says can't be
// believed.
func TestCheckPrivate(t *testing.T) {
	local := Transport{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}
	tests := []struct {
		name   string
		tr     Transport
		remote string
		header http.Header
		want   error
	}{
		{"a browser on this machine", Transport{}, "127.0.0.1:4000", nil, nil},
		{"an unlisted proxy on this machine", Transport{}, "127.0.0.1:4000",
			http.Header{"X-Forwarded-For": {"198.51.100.7"}, "X-Forwarded-Proto": {"https"}}, errUnlistedProxy},
		{"an unlisted proxy, even with plain HTTP allowed", Transport{AllowPlainHTTP: true}, "127.0.0.1:4000",
			http.Header{"Cf-Connecting-Ip": {"198.51.100.7"}}, errUnlistedProxy},
		{"a listed proxy saying HTTPS", local, "127.0.0.1:4000",
			http.Header{"X-Forwarded-For": {"198.51.100.7"}, "X-Forwarded-Proto": {"https"}}, nil},
		{"a listed proxy saying plain HTTP", local, "127.0.0.1:4000",
			http.Header{"X-Forwarded-For": {"198.51.100.7"}, "X-Forwarded-Proto": {"http"}}, errPlainHTTP},
		{"the nearest proxy's protocol counts", local, "127.0.0.1:4000",
			http.Header{"X-Forwarded-For": {"198.51.100.7"}, "X-Forwarded-Proto": {"https", "https, http"}}, errPlainHTTP},
		{"a browser beside a listed proxy", local, "127.0.0.1:4000", nil, nil},
		{"another machine over plain HTTP", Transport{}, "198.51.100.7:4000", nil, errPlainHTTP},
		{"another machine, plain HTTP allowed", Transport{AllowPlainHTTP: true}, "198.51.100.7:4000", nil, nil},
	}
	for _, tt := range tests {
		if got := tt.tr.CheckPrivate(request(tt.remote, tt.header)); !errors.Is(got, tt.want) {
			t.Errorf("%s: CheckPrivate = %v, want %v", tt.name, got, tt.want)
		}
	}
}
