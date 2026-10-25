package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tensora/tensora/controlplane/internal/credential"
	"github.com/tensora/tensora/controlplane/internal/domain"
	"github.com/tensora/tensora/controlplane/internal/store"
)

// Local accounts (ADR-0012): one owner account today; invites and teams arrive in Phase 2.

const (
	// SessionTTL is how long a browser stays signed in.
	SessionTTL = 7 * 24 * time.Hour
	// TokenPrefix marks API tokens, so they're recognizable (and findable by secret scanners).
	TokenPrefix = "tsk_"
	// maxTokenTTL bounds an API token's lifetime; zero means it doesn't expire.
	maxTokenTTL = 366 * 24 * time.Hour
	// ownerQuota is the owner's quota on every class at setup: it's their machine.
	ownerQuota = 64
	// touchEvery limits how often a token's last-used time is written, so API calls stay reads.
	touchEvery = time.Minute
)

// SetupInput creates the first account: the machine's owner.
type SetupInput struct {
	Email      string
	Password   string
	TenantSlug string // the owner's tenant; created as trusted if it doesn't exist
	TenantName string
}

// SetupOwner creates the first account, its trusted tenant, and a quota on every class. It refuses
// once any account exists, so it can't be used to take over an installation. It's called by
// `tensora setup`, which runs on the machine itself with access to the store.
func (s *Service) SetupOwner(ctx context.Context, in SetupInput) (domain.User, error) {
	email, ok := domain.NormalizeEmail(in.Email)
	if !ok {
		return domain.User{}, domain.Errorf(domain.CodeInvalid, "that doesn't look like an email address")
	}
	if err := credential.CheckNewPassword(in.Password, email); err != nil {
		return domain.User{}, domain.Errorf(domain.CodeInvalid, "%v", err)
	}
	if !domain.ValidSlug(in.TenantSlug) {
		return domain.User{}, domain.Errorf(domain.CodeInvalid, "tenant must be 1-40 lowercase letters, digits, or '-'")
	}
	var hash string
	var hashErr error
	if err := s.withHashSlot(ctx, func() { hash, hashErr = credential.HashPassword(in.Password) }); err != nil {
		return domain.User{}, err // slow on purpose, so outside any transaction
	}
	if hashErr != nil {
		return domain.User{}, hashErr
	}
	now := s.now().UTC()
	u := domain.User{ID: uuid.Must(uuid.NewV7()), Email: email, PasswordHash: hash, CreatedAt: now}
	err := s.store.InTx(ctx, func(q store.Queries) error {
		if err := q.LockUsers(ctx); err != nil {
			return err
		}
		if n, err := q.CountUsers(ctx); err != nil || n > 0 {
			if err != nil {
				return err
			}
			return domain.Errorf(domain.CodeConflict, "an account already exists; setup only creates the first one")
		}
		t, err := q.TenantBySlug(ctx, in.TenantSlug)
		switch {
		case domain.IsNotFound(err):
			name := in.TenantName
			if name == "" {
				name = in.TenantSlug
			}
			t = domain.Tenant{ID: uuid.Must(uuid.NewV7()), Slug: in.TenantSlug, Name: name, Tier: domain.TierTrusted, CreatedAt: now}
			if err := q.CreateTenant(ctx, t); err != nil {
				return err
			}
			for _, g := range s.catalog.GPUClasses {
				if err := q.SetQuota(ctx, domain.Quota{TenantID: t.ID, GPUClass: g.ID, MaxGPUs: ownerQuota}); err != nil {
					return err
				}
			}
		case err != nil:
			return err
		case t.Tier != domain.TierTrusted:
			return domain.Errorf(domain.CodeConflict, "tenant %q exists and isn't trusted, so it can't hold the owner", in.TenantSlug)
		}
		u.TenantID = t.ID
		return q.CreateUser(ctx, u)
	})
	return u, err
}

// Sign-in limits (per 15 minutes). The strict one is per email and caller pair, so nobody can lock
// the owner out just by knowing their email; the looser one per caller address bounds guessing
// across emails. Together with hashSlots, which caps how fast passwords are checked at all, online
// guessing of a 12+ character password isn't practical.
const (
	maxPairFailures   = 10
	maxClientFailures = 30
)

// errWrongPassword is the same for an unknown email and a wrong password, so sign-in doesn't
// reveal which accounts exist.
var errWrongPassword = domain.Errorf(domain.CodeUnauthenticated, "wrong email or password")

// withHashSlot runs fn, which hashes a password, once a hashing slot is free; it gives up after a
// few seconds rather than queueing without bound.
func (s *Service) withHashSlot(ctx context.Context, fn func()) error {
	wait := time.NewTimer(3 * time.Second)
	defer wait.Stop()
	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
		fn()
		return nil
	case <-wait.C:
		return domain.Errorf(domain.CodeTooManyRequests, "the server is busy checking other passwords; try again in a moment")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Login checks an email and password and starts a session, returning the session token to put in
// a cookie. client identifies the caller (its address) for slowing down guessing.
func (s *Service) Login(ctx context.Context, email, password, client string) (string, error) {
	dummy := s.dummy.get() // made before anything else, so the first sign-in costs the same for every email
	now := s.now().UTC()
	email, emailOK := domain.NormalizeEmail(email)
	pair, perClient := "pair:"+email+"|"+client, "client:"+client
	wait, ok := s.logins.begin(now, limit{pair, maxPairFailures}, limit{perClient, maxClientFailures})
	if !ok {
		return "", domain.Errorf(domain.CodeTooManyRequests, "too many failed sign-ins; try again in %d minute(s)", int(wait.Minutes())+1)
	}
	failed := true // until the password checks out; counted when the attempt ends
	defer func() { s.logins.end(s.now().UTC(), failed, pair, perClient) }()
	if len(password) > credential.MaxPasswordBytes {
		return "", errWrongPassword
	}

	var u domain.User
	found := false
	if emailOK {
		err := s.store.InTx(ctx, func(q store.Queries) error {
			var err error
			u, err = q.UserByEmail(ctx, email)
			return err
		})
		switch {
		case err == nil:
			found = true
		case !domain.IsNotFound(err):
			failed = false
			return "", err
		}
	}
	// Unknown emails are checked against the dummy hash, at the same cost as real ones. Hashing
	// happens outside any transaction: it's slow on purpose.
	hash := dummy
	if found {
		hash = u.PasswordHash
	}
	matched := false
	if err := s.withHashSlot(ctx, func() { matched = credential.VerifyPassword(hash, password) }); err != nil {
		failed = false // busy, not a wrong guess
		return "", err
	}
	if !matched || !found {
		return "", errWrongPassword
	}
	failed = false
	if !u.DisabledAt.IsZero() {
		return "", domain.Errorf(domain.CodeForbidden, "this account is disabled")
	}
	s.logins.reset(pair)

	// Hashes made with older parameters are upgraded, so every account costs the same to check.
	rehashed := ""
	if credential.NeedsRehash(u.PasswordHash) {
		_ = s.withHashSlot(ctx, func() { rehashed, _ = credential.HashPassword(password) }) // best effort
	}
	token, tokenHash, err := credential.NewToken("")
	if err != nil {
		return "", err
	}
	err = s.store.InTx(ctx, func(q store.Queries) error {
		// The password was checked outside this transaction. If it changed meanwhile, or the
		// account was disabled, the old password no longer opens a session.
		current, err := q.UserByID(ctx, u.ID)
		if err != nil {
			return err
		}
		if current.PasswordHash != u.PasswordHash || !current.DisabledAt.IsZero() {
			return errWrongPassword
		}
		if rehashed != "" {
			if err := q.SetPasswordHash(ctx, u.ID, rehashed); err != nil {
				return err
			}
		}
		if err := q.DeleteExpiredSessions(ctx, now); err != nil {
			return err
		}
		return q.CreateSession(ctx, domain.Session{TokenHash: tokenHash, UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(SessionTTL)})
	})
	return token, err
}

// Logout ends the session with token. Ending one that's already gone is fine.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.store.InTx(ctx, func(q store.Queries) error { return q.DeleteSession(ctx, credential.HashToken(token)) })
}

var errSignIn = domain.Errorf(domain.CodeUnauthenticated, "sign in again: the session has ended")

// PrincipalForSession resolves a browser's session token.
func (s *Service) PrincipalForSession(ctx context.Context, token string) (domain.Principal, error) {
	now := s.now().UTC()
	var p domain.Principal
	err := s.store.InTx(ctx, func(q store.Queries) error {
		sess, err := q.SessionByTokenHash(ctx, credential.HashToken(token))
		if domain.IsNotFound(err) {
			return errSignIn
		}
		if err != nil {
			return err
		}
		if !now.Before(sess.ExpiresAt) {
			return errSignIn
		}
		p, err = principalFor(ctx, q, sess.UserID)
		p.Via, p.Credential = domain.AuthSession, "browser session"
		return err
	})
	return p, err
}

// PrincipalForToken resolves an API token (TokenPrefix…).
func (s *Service) PrincipalForToken(ctx context.Context, token string) (domain.Principal, error) {
	invalid := domain.Errorf(domain.CodeUnauthenticated, "invalid, expired, or revoked API token")
	if len(token) < len(TokenPrefix)+16 || token[:len(TokenPrefix)] != TokenPrefix {
		return domain.Principal{}, invalid
	}
	now := s.now().UTC()
	var p domain.Principal
	err := s.store.InTx(ctx, func(q store.Queries) error {
		t, err := q.APITokenByHash(ctx, credential.HashToken(token))
		if domain.IsNotFound(err) {
			return invalid
		}
		if err != nil {
			return err
		}
		if !t.Usable(now) {
			return invalid
		}
		if now.Sub(t.LastUsedAt) >= touchEvery {
			if err := q.TouchAPIToken(ctx, t.ID, now); err != nil {
				return err
			}
		}
		p, err = principalFor(ctx, q, t.UserID)
		p.Via, p.Credential = domain.AuthToken, fmt.Sprintf("API token %q (%s…)", t.Name, t.Prefix)
		return err
	})
	return p, err
}

// principalFor builds the identity of an active user; a disabled one can't authenticate.
func principalFor(ctx context.Context, q store.Queries, userID uuid.UUID) (domain.Principal, error) {
	u, err := q.UserByID(ctx, userID)
	if err != nil {
		return domain.Principal{}, err
	}
	if !u.DisabledAt.IsZero() {
		return domain.Principal{}, domain.Errorf(domain.CodeUnauthenticated, "this account is disabled")
	}
	t, err := q.TenantByID(ctx, u.TenantID)
	if err != nil {
		return domain.Principal{}, err
	}
	return domain.Principal{Subject: u.Email, Email: u.Email, TenantSlug: t.Slug, UserID: u.ID}, nil
}

func requireAccount(p domain.Principal) error {
	if p.UserID == uuid.Nil {
		return domain.Errorf(domain.CodeInvalid, "this needs a local account (TENSORA_AUTH_MODE=local)")
	}
	return nil
}

// requireSession refuses an API token for things that would let a stolen token outlive its
// revocation: changing the password, and making more tokens. Those need a signed-in browser.
func requireSession(p domain.Principal) error {
	if err := requireAccount(p); err != nil {
		return err
	}
	if p.Via != domain.AuthSession {
		return domain.Errorf(domain.CodeForbidden, "this needs a signed-in browser, not an API token")
	}
	return nil
}

// ChangePassword checks the current password, sets a new one, and signs the user out of every
// other session. currentSession is the caller's session token, which stays signed in. It returns
// how many API tokens still work, so the user can revoke them too if the password leaked.
func (s *Service) ChangePassword(ctx context.Context, p domain.Principal, current, next, currentSession, client string) (int, error) {
	if err := requireSession(p); err != nil {
		return 0, err
	}
	now := s.now().UTC()
	byUser, perClient := "password:"+p.UserID.String(), "client:"+client
	wait, ok := s.logins.begin(now, limit{byUser, maxPairFailures}, limit{perClient, maxClientFailures})
	if !ok {
		return 0, domain.Errorf(domain.CodeTooManyRequests, "too many wrong passwords; try again in %d minute(s)", int(wait.Minutes())+1)
	}
	failed := true
	defer func() { s.logins.end(s.now().UTC(), failed, byUser, perClient) }()
	if len(current) > credential.MaxPasswordBytes {
		return 0, domain.Errorf(domain.CodeForbidden, "the current password is wrong")
	}
	var u domain.User
	if err := s.store.InTx(ctx, func(q store.Queries) error {
		var err error
		u, err = q.UserByID(ctx, p.UserID)
		return err
	}); err != nil {
		failed = false
		return 0, err
	}
	matched := false
	if err := s.withHashSlot(ctx, func() { matched = credential.VerifyPassword(u.PasswordHash, current) }); err != nil {
		failed = false
		return 0, err
	}
	if !matched {
		return 0, domain.Errorf(domain.CodeForbidden, "the current password is wrong")
	}
	failed = false
	if err := credential.CheckNewPassword(next, u.Email); err != nil {
		return 0, domain.Errorf(domain.CodeInvalid, "%v", err)
	}
	var hash string
	var hashErr error
	if err := s.withHashSlot(ctx, func() { hash, hashErr = credential.HashPassword(next) }); err != nil {
		return 0, err
	}
	if hashErr != nil {
		return 0, hashErr
	}
	live := 0
	err := s.store.InTx(ctx, func(q store.Queries) error {
		latest, err := q.UserByID(ctx, u.ID)
		if err != nil {
			return err
		}
		if latest.PasswordHash != u.PasswordHash {
			return domain.Errorf(domain.CodeConflict, "the password changed meanwhile; sign in again")
		}
		if err := q.SetPasswordHash(ctx, u.ID, hash); err != nil {
			return err
		}
		if err := q.DeleteOtherSessions(ctx, u.ID, credential.HashToken(currentSession)); err != nil {
			return err
		}
		tokens, err := q.ListAPITokens(ctx, u.ID)
		for _, t := range tokens {
			if t.Usable(now) {
				live++
			}
		}
		return err
	})
	return live, err
}

// ResetPassword sets a new password for an existing account and signs it out everywhere. It's
// for `tensora setup -reset-password`, which needs access to the store: the way back in for an
// owner who forgot their password, since there's no email reset.
func (s *Service) ResetPassword(ctx context.Context, email, password string) (domain.User, error) {
	email, ok := domain.NormalizeEmail(email)
	if !ok {
		return domain.User{}, domain.Errorf(domain.CodeInvalid, "that doesn't look like an email address")
	}
	if err := credential.CheckNewPassword(password, email); err != nil {
		return domain.User{}, domain.Errorf(domain.CodeInvalid, "%v", err)
	}
	var hash string
	var hashErr error
	if err := s.withHashSlot(ctx, func() { hash, hashErr = credential.HashPassword(password) }); err != nil {
		return domain.User{}, err
	}
	if hashErr != nil {
		return domain.User{}, hashErr
	}
	var u domain.User
	err := s.store.InTx(ctx, func(q store.Queries) error {
		var err error
		u, err = q.UserByEmail(ctx, email)
		if domain.IsNotFound(err) {
			return domain.Errorf(domain.CodeNotFound, "no account has the email %s", email)
		}
		if err != nil {
			return err
		}
		if err := q.SetPasswordHash(ctx, u.ID, hash); err != nil {
			return err
		}
		return q.DeleteOtherSessions(ctx, u.ID, "")
	})
	return u, err
}

// CreateAPIToken makes a token for the caller. The token is returned once and never stored; ttl
// zero means it doesn't expire.
func (s *Service) CreateAPIToken(ctx context.Context, p domain.Principal, name string, ttl time.Duration) (string, domain.APIToken, error) {
	if err := requireSession(p); err != nil {
		return "", domain.APIToken{}, err
	}
	if !domain.ValidTokenName(name) {
		return "", domain.APIToken{}, domain.Errorf(domain.CodeInvalid, "a token name is 1-64 printable characters")
	}
	if ttl < 0 || ttl > maxTokenTTL {
		return "", domain.APIToken{}, domain.Errorf(domain.CodeInvalid, "a token can last at most 366 days (or not expire)")
	}
	token, hash, err := credential.NewToken(TokenPrefix)
	if err != nil {
		return "", domain.APIToken{}, err
	}
	now := s.now().UTC()
	t := domain.APIToken{ID: uuid.Must(uuid.NewV7()), UserID: p.UserID, Name: name, Prefix: token[:len(TokenPrefix)+4],
		TokenHash: hash, CreatedAt: now}
	if ttl > 0 {
		t.ExpiresAt = now.Add(ttl)
	}
	err = s.store.InTx(ctx, func(q store.Queries) error { return q.CreateAPIToken(ctx, t) })
	if code, _ := domain.CodeOf(err); code == domain.CodeConflict {
		return "", domain.APIToken{}, domain.Errorf(domain.CodeConflict, "you already have a token named %q", name)
	}
	return token, t, err
}

// ListAPITokens lists the caller's own tokens, including revoked and expired ones.
func (s *Service) ListAPITokens(ctx context.Context, p domain.Principal) ([]domain.APIToken, error) {
	if err := requireAccount(p); err != nil {
		return nil, err
	}
	var out []domain.APIToken
	err := s.store.InTx(ctx, func(q store.Queries) error {
		var err error
		out, err = q.ListAPITokens(ctx, p.UserID)
		return err
	})
	return out, err
}

// RevokeAPIToken revokes one of the caller's tokens. Another user's token is not found.
func (s *Service) RevokeAPIToken(ctx context.Context, p domain.Principal, id uuid.UUID) error {
	if err := requireAccount(p); err != nil {
		return err
	}
	return s.store.InTx(ctx, func(q store.Queries) error { return q.RevokeAPIToken(ctx, p.UserID, id, s.now().UTC()) })
}

// dummyHash is a real argon2id hash of nothing anyone knows, made once on first use, for checking
// unknown emails at the same cost as real ones.
type dummyHash struct {
	once sync.Once
	hash string
}

func (d *dummyHash) get() string {
	d.once.Do(func() {
		token, _, err := credential.NewToken("")
		if err != nil {
			token = "unguessable"
		}
		d.hash, _ = credential.HashPassword(token) // on error, the empty hash never matches
	})
	return d.hash
}

// loginLimiter slows down password guessing. An attempt is counted as it starts, so attempts made
// in parallel can't slip past the limit while each is still being checked.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	inFlight map[string]int
}

type limit struct {
	key string
	max int
}

const (
	loginWindow    = 15 * time.Minute
	maxLimiterKeys = 10000
)

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: map[string][]time.Time{}, inFlight: map[string]int{}}
}

func (l *loginLimiter) recent(key string, now time.Time) []time.Time {
	var kept []time.Time
	for _, t := range l.failures[key] {
		if now.Sub(t) < loginWindow {
			kept = append(kept, t)
		}
	}
	return kept
}

// begin reserves an attempt under every limit, or reports how long to wait when recent failures
// plus attempts in progress have reached one.
func (l *loginLimiter) begin(now time.Time, limits ...limit) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, lim := range limits {
		r := l.recent(lim.key, now)
		if len(r)+l.inFlight[lim.key] >= lim.max {
			if len(r) == 0 {
				return time.Second, false // only attempts in progress: try again shortly
			}
			return loginWindow - now.Sub(r[0]), false
		}
	}
	for _, lim := range limits {
		l.inFlight[lim.key]++
	}
	return 0, true
}

// end finishes an attempt begun with the same keys, recording a failure if it failed.
func (l *loginLimiter) end(now time.Time, failed bool, keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.failures) > maxLimiterKeys { // bound memory: forget keys with nothing recent
		for k := range l.failures {
			if len(l.recent(k, now)) == 0 {
				delete(l.failures, k)
			}
		}
	}
	for _, k := range keys {
		if l.inFlight[k]--; l.inFlight[k] <= 0 {
			delete(l.inFlight, k)
		}
		if failed {
			l.failures[k] = append(l.recent(k, now), now)
		}
	}
}

func (l *loginLimiter) reset(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		delete(l.failures, k)
	}
}

// String keeps the password out of logs if a SetupInput is ever printed by mistake.
func (SetupInput) String() string { return "SetupInput{redacted}" }
