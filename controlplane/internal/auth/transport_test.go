package auth

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

// Client is who's guessing, for the sign-in limits: a forwarded address is believed only from a
// trusted proxy, and an IPv6 address counts as its /64.
func TestClient(t *testing.T) {
	tr := Transport{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")}}
	tests := []struct {
		name, remote, forwarded, want string
	}{
		{"direct", "198.51.100.7:4000", "", "198.51.100.7"},
		{"forwarded by an untrusted peer is ignored", "198.51.100.7:4000", "203.0.113.9", "198.51.100.7"},
		{"through a trusted proxy", "10.0.0.5:4000", "203.0.113.9", "203.0.113.9"},
		{"the nearest untrusted hop, not a spoofed first one", "10.0.0.5:4000", "1.2.3.4, 203.0.113.9, 10.0.0.6", "203.0.113.9"},
		{"a garbled hop stops the walk", "10.0.0.5:4000", "203.0.113.9, nonsense", "10.0.0.5"},
		{"a trusted proxy with no header", "10.0.0.5:4000", "", "10.0.0.5"},
		{"IPv6 counts as its /64", "[2001:db8:1:2:3:4:5:6]:4000", "", "2001:db8:1:2::/64"},
		{"IPv4-mapped IPv6 is IPv4", "[::ffff:198.51.100.7]:4000", "", "198.51.100.7"},
	}
	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tt.remote
		if tt.forwarded != "" {
			r.Header.Set("X-Forwarded-For", tt.forwarded)
		}
		if got := tr.Client(r); got != tt.want {
			t.Errorf("%s: Client = %q, want %q", tt.name, got, tt.want)
		}
	}
}
