package service

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// The limiter's memory is capped: when full, it forgets the least recently failed keys, pair keys
// before per-caller ones, which still bound guessing.
func TestLimiterIsCapped(t *testing.T) {
	l := newLoginLimiter()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	fail := func(key string) {
		if _, ok := l.begin(now, limit{key: key, max: 10}); ok {
			l.end(now, true, key)
		}
	}
	fail("client:198.51.100.7")
	for i := range maxLimiterKeys + 500 {
		now = now.Add(time.Millisecond)
		fail(fmt.Sprintf("pair:user%d@example.com|203.0.113.9", i))
	}
	if n := len(l.failures); n > maxLimiterKeys {
		t.Fatalf("%d keys kept; the cap is %d", n, maxLimiterKeys)
	}
	if _, ok := l.failures["client:198.51.100.7"]; !ok {
		t.Error("a per-caller key was forgotten before pair keys")
	}
	for k := range l.failures {
		if strings.HasSuffix(k, "user0@example.com|203.0.113.9") {
			t.Error("the oldest pair key was kept")
		}
	}

	// Keys with nothing recent are forgotten at the next sweep.
	now = now.Add(loginWindow + sweepEvery)
	fail("pair:late@example.com|203.0.113.9")
	if n := len(l.failures); n != 1 {
		t.Errorf("%d keys after the window; want only the new one", n)
	}
}

func TestLimiterKeys(t *testing.T) {
	for client, want := range map[string][2]string{
		"203.0.113.9":           {"203.0.113.9", ""},
		"2001:db8:1:2:3:4:5:6":  {"2001:db8:1:2::/64", "2001:db8:1::/48"},
		"not an address at all": {"not an address at all", ""},
	} {
		if caller, network := limiterKeys(client); caller != want[0] || network != want[1] {
			t.Errorf("limiterKeys(%q) = %q, %q; want %q", client, caller, network, want)
		}
	}
}

// Moving between the /64s of one IPv6 /48, which one person can easily hold, only goes so far; a
// neighbour's /64 has its own, stricter budget.
func TestLimiterBoundsAnIPv6Network(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	s := &Service{logins: newLoginLimiter(), now: func() time.Time { return now }}
	for i := range maxNetworkFailures {
		end, err := s.beginAttempt(now, fmt.Sprintf("guess%d@example.com", i), fmt.Sprintf("2001:db8:9:%x::1", i/25))
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		end(true)
	}
	if _, err := s.beginAttempt(now, "owner@example.com", "2001:db8:9:ff::1"); err == nil {
		t.Error("a fresh /64 in a /48 that has used its budget was let through")
	}
	if _, err := s.beginAttempt(now, "owner@example.com", "2001:db8:a::1"); err != nil {
		t.Errorf("another /48: %v", err)
	}
}
