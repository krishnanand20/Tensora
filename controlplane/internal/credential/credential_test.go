package credential

import (
	"strings"
	"testing"
)

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("hash = %q", hash)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Error("the right password must verify")
	}
	if VerifyPassword(hash, "correct horse battery stapl") {
		t.Error("a wrong password must not verify")
	}
	again, _ := HashPassword("correct horse battery staple")
	if again == hash {
		t.Error("each hash needs its own salt")
	}
}

func TestMalformedHashesNeverMatch(t *testing.T) {
	good, _ := HashPassword("correct horse battery staple")
	parts := strings.Split(good, "$")
	for name, hash := range map[string]string{
		"empty":           "",
		"bcrypt":          "$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyzabcde",
		"argon2i":         strings.Replace(good, "argon2id", "argon2i", 1),
		"huge memory":     strings.Join(append(parts[:3:3], "m=4194304,t=3,p=2", parts[4], parts[5]), "$"),
		"many passes":     strings.Join(append(parts[:3:3], "m=65536,t=99,p=2", parts[4], parts[5]), "$"),
		"bad salt":        strings.Join(append(parts[:4:4], "!!!", parts[5]), "$"),
		"truncated parts": strings.Join(parts[:5], "$"),
	} {
		if VerifyPassword(hash, "correct horse battery staple") {
			t.Errorf("%s: a malformed hash matched", name)
		}
	}
}

func TestTokens(t *testing.T) {
	token, hash, err := NewToken("tsk_")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "tsk_") || len(token) < 40 {
		t.Errorf("token = %q", token)
	}
	if hash != HashToken(token) || len(hash) != 64 || strings.Contains(hash, token) {
		t.Errorf("hash = %q; want the hex SHA-256 of the token, never the token", hash)
	}
	other, _, _ := NewToken("tsk_")
	if other == token {
		t.Error("tokens must be random")
	}
}

func TestCheckNewPassword(t *testing.T) {
	for pw, ok := range map[string]bool{
		"correct horse battery staple": true,
		"short":                        false,
		"elevenchars":                  false,
		"twelve chars":                 true,
		"me@example.com":               false, // equals the email
		strings.Repeat("x", 1025):      false,
		"Password1234":                 false, // common, whatever the case
		"password-1234!":               false, // common, whatever the punctuation
		"aaaaaaaaaaaaaaaa":             false, // mostly one character
		"abababababab":                 false,
		"glacier tulip 42 obey":        true,
	} {
		if err := CheckNewPassword(pw, "me@example.com"); (err == nil) != ok {
			t.Errorf("CheckNewPassword(%.20q) = %v, want ok=%v", pw, err, ok)
		}
	}
}
