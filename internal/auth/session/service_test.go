package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/auth"
	"github.com/soltiHQ/control-plane/internal/auth/credentials"
	"github.com/soltiHQ/control-plane/internal/auth/identity"
	"github.com/soltiHQ/control-plane/internal/auth/providers"
	passwordprovider "github.com/soltiHQ/control-plane/internal/auth/providers/password"
	"github.com/soltiHQ/control-plane/internal/auth/token"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/storage/inmemory"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.t = c.t.Add(d)
}

type fakeIssuer struct {
	last *identity.Identity
	out  string
	err  error
}

func (i *fakeIssuer) Issue(ctx context.Context, id *identity.Identity) (string, error) {
	i.last = id
	if i.err != nil {
		return "", i.err
	}
	if i.out != "" {
		return i.out, nil
	}
	return "access-token", nil
}

type fakeRBAC struct {
	perms []enum.Permission
	err   error
}

func (r *fakeRBAC) ResolveUserPermissions(ctx context.Context, u *model.User) ([]enum.Permission, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make([]enum.Permission, len(r.perms))
	copy(out, r.perms)
	return out, nil
}

type badProvider struct{}

func (badProvider) Kind() enum.Auth { return enum.APIKey }
func (badProvider) Authenticate(ctx context.Context, req providers.Request) (*providers.Result, error) {
	return nil, auth.ErrInvalidRequest
}

func mustUser(t *testing.T, id, subject string, disabled bool) *model.User {
	t.Helper()
	u, err := model.NewUser(id, subject)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if disabled {
		u.Disable()
	}
	return u
}

func mustUpsertUser(t *testing.T, ctx context.Context, st storage.Storage, u *model.User) {
	t.Helper()
	if err := st.UpsertUser(ctx, u); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
}

func mustUpsertPasswordCred(t *testing.T, ctx context.Context, st storage.Storage, credID, userID, plain string) *model.Credential {
	t.Helper()
	cred, err := credentials.NewPasswordCredential(credID, userID)
	if err != nil {
		t.Fatalf("NewPasswordCredential: %v", err)
	}
	if err := st.UpsertCredential(ctx, cred); err != nil {
		t.Fatalf("UpsertCredential: %v", err)
	}
	ver, err := credentials.NewPasswordVerifier(credID+"-ver", credID, plain, credentials.DefaultBcryptCost)
	if err != nil {
		t.Fatalf("NewPasswordVerifier: %v", err)
	}
	if err := st.UpsertVerifier(ctx, ver); err != nil {
		t.Fatalf("UpsertVerifier: %v", err)
	}
	return cred
}

func newService(t *testing.T, st storage.Storage, clk token.Clock, iss token.Issuer, rbac RBACResolver, rotate bool) *Service {
	t.Helper()

	cfg := Config{
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    24 * time.Hour,
		Issuer:        "solti",
		Audience:      "control-plane",
		RotateRefresh: rotate,
	}

	provs := map[enum.Auth]providers.Provider{
		enum.Password: passwordprovider.New(st),
	}

	return New(st, iss, clk, cfg, rbac, provs)
}

func TestService_EnsureReady_InvalidWiring(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()

	clk := &fakeClock{t: time.Unix(1, 0)}
	iss := &fakeIssuer{}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a"}}

	t.Run("nil service", func(t *testing.T) {
		var s *Service
		_, _, err := s.Login(ctx, enum.Password, "s", "p")
		if !errors.Is(err, auth.ErrInvalidRequest) {
			t.Fatalf("expected ErrInvalidRequest, got %v", err)
		}
	})

	t.Run("nil store", func(t *testing.T) {
		s := New(nil, iss, clk, Config{}, rbac, nil)
		_, _, err := s.Login(ctx, enum.Password, "s", "p")
		if !errors.Is(err, auth.ErrInvalidRequest) {
			t.Fatalf("expected ErrInvalidRequest, got %v", err)
		}
	})

	t.Run("nil issuer", func(t *testing.T) {
		s := New(st, nil, clk, Config{}, rbac, nil)
		_, _, err := s.Login(ctx, enum.Password, "s", "p")
		if !errors.Is(err, auth.ErrInvalidRequest) {
			t.Fatalf("expected ErrInvalidRequest, got %v", err)
		}
	})

	t.Run("nil rbac", func(t *testing.T) {
		s := New(st, iss, clk, Config{}, nil, nil)
		_, _, err := s.Login(ctx, enum.Password, "s", "p")
		if !errors.Is(err, auth.ErrInvalidRequest) {
			t.Fatalf("expected ErrInvalidRequest, got %v", err)
		}
	})
}

func TestService_Login_InvalidInput(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(1, 0)}
	iss := &fakeIssuer{}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a"}}

	s := newService(t, st, clk, iss, rbac, true)

	t.Run("empty subject", func(t *testing.T) {
		_, _, err := s.Login(ctx, enum.Password, "", "pw")
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("empty secret", func(t *testing.T) {
		_, _, err := s.Login(ctx, enum.Password, "subj", "")
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("unsupported auth kind", func(t *testing.T) {
		_, _, err := s.Login(ctx, enum.APIKey, "subj", "pw")
		if !errors.Is(err, auth.ErrInvalidRequest) {
			t.Fatalf("expected ErrInvalidRequest, got %v", err)
		}
	})
}

func TestService_Login_ProviderMappingValidation(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(1, 0)}
	iss := &fakeIssuer{}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a"}}

	s := New(
		st,
		iss,
		clk,
		Config{AccessTTL: time.Minute, RefreshTTL: time.Hour, Issuer: "i", Audience: "a", RotateRefresh: true},
		rbac,
		map[enum.Auth]providers.Provider{
			enum.Password: badProvider{},
		},
	)

	_, _, err := s.Login(ctx, enum.Password, "subj", "pw")
	if !errors.Is(err, auth.ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestService_Login_UnauthorizedWhenNoPerms(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(1, 0)}
	iss := &fakeIssuer{}
	rbac := &fakeRBAC{perms: nil}

	u := mustUser(t, "u1", "subj-1", false)
	mustUpsertUser(t, ctx, st, u)
	_ = mustUpsertPasswordCred(t, ctx, st, "cred-1", u.ID(), "pw")

	s := newService(t, st, clk, iss, rbac, true)

	_, _, err := s.Login(ctx, enum.Password, "subj-1", "pw")
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Login_Success_CreatesSession_IssuesToken(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(100, 0)}
	iss := &fakeIssuer{out: "access-1"}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a", "perm:b"}}

	u := mustUser(t, "u1", "subj-1", false)
	u.SetName("User One")
	u.SetEmail("u1@example.com")
	mustUpsertUser(t, ctx, st, u)
	cred := mustUpsertPasswordCred(t, ctx, st, "cred-1", u.ID(), "pw")

	s := newService(t, st, clk, iss, rbac, true)

	pair, id, err := s.Login(ctx, enum.Password, "subj-1", "pw")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if pair == nil || pair.AccessToken != "access-1" || pair.RefreshToken == "" {
		t.Fatalf("unexpected token pair: %#v", pair)
	}
	if id == nil {
		t.Fatal("expected non-nil identity")
	}
	if id.UserID != u.ID() || id.Subject != u.Subject() || id.SessionID == "" || id.TokenID == "" {
		t.Fatalf("unexpected identity: %#v", id)
	}
	if id.Issuer != s.cfg.Issuer || len(id.Audience) != 1 || id.Audience[0] != s.cfg.Audience {
		t.Fatalf("unexpected issuer/audience in identity: %#v", id)
	}
	if id.IssuedAt != clk.Now() || id.NotBefore != clk.Now() {
		t.Fatalf("unexpected iat/nbf: iat=%v nbf=%v now=%v", id.IssuedAt, id.NotBefore, clk.Now())
	}
	if id.ExpiresAt != clk.Now().Add(s.cfg.AccessTTL) {
		t.Fatalf("unexpected exp: got %v want %v", id.ExpiresAt, clk.Now().Add(s.cfg.AccessTTL))
	}
	if id.Permissions == nil || len(id.Permissions) != 2 {
		t.Fatalf("unexpected perms: %#v", id.Permissions)
	}

	sess, err := st.GetSession(ctx, id.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.UserID() != u.ID() {
		t.Fatalf("session user mismatch: %q != %q", sess.UserID(), u.ID())
	}
	if sess.CredentialID() != cred.ID() {
		t.Fatalf("session cred mismatch: %q != %q", sess.CredentialID(), cred.ID())
	}
	if sess.AuthKind() != enum.Password {
		t.Fatalf("session auth kind mismatch: %q", sess.AuthKind())
	}
	if len(sess.RefreshHash()) == 0 {
		t.Fatalf("expected stored refresh hash")
	}
}

func TestService_Refresh_InvalidInput(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(1, 0)}
	iss := &fakeIssuer{}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a"}}

	s := newService(t, st, clk, iss, rbac, true)

	_, _, err := s.Refresh(ctx, "", "raw")
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("expected ErrInvalidRefresh, got %v", err)
	}
	_, _, err = s.Refresh(ctx, "sid", "")
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("expected ErrInvalidRefresh, got %v", err)
	}
}

func TestService_Refresh_SessionNotFound(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(1, 0)}
	iss := &fakeIssuer{}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a"}}

	s := newService(t, st, clk, iss, rbac, true)

	_, _, err := s.Refresh(ctx, "nope", "raw")
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("expected ErrInvalidRefresh, got %v", err)
	}
}

func TestService_Refresh_Success_NoRotate(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(100, 0)}
	iss := &fakeIssuer{out: "access-2"}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a"}}

	u := mustUser(t, "u1", "subj-1", false)
	mustUpsertUser(t, ctx, st, u)
	_ = mustUpsertPasswordCred(t, ctx, st, "cred-1", u.ID(), "pw")

	s := newService(t, st, clk, iss, rbac, false)

	pair, id, err := s.Login(ctx, enum.Password, "subj-1", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	oldRefresh := pair.RefreshToken
	sid := id.SessionID

	clk.Advance(1 * time.Minute)

	pair2, id2, err := s.Refresh(ctx, sid, oldRefresh)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if pair2.RefreshToken != oldRefresh {
		t.Fatalf("expected refresh token unchanged, got %q want %q", pair2.RefreshToken, oldRefresh)
	}
	if pair2.AccessToken != "access-2" {
		t.Fatalf("unexpected access token: %q", pair2.AccessToken)
	}
	if id2.SessionID != sid {
		t.Fatalf("session id mismatch: %q != %q", id2.SessionID, sid)
	}
}

func TestService_Refresh_Success_RotateAndOldTokenStopsWorking(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(200, 0)}
	iss := &fakeIssuer{out: "access-3"}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a"}}

	u := mustUser(t, "u1", "subj-1", false)
	mustUpsertUser(t, ctx, st, u)
	_ = mustUpsertPasswordCred(t, ctx, st, "cred-1", u.ID(), "pw")

	s := newService(t, st, clk, iss, rbac, true)

	pair, id, err := s.Login(ctx, enum.Password, "subj-1", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	oldRefresh := pair.RefreshToken
	sid := id.SessionID

	clk.Advance(1 * time.Minute)

	pair2, _, err := s.Refresh(ctx, sid, oldRefresh)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if pair2.RefreshToken == "" || pair2.RefreshToken == oldRefresh {
		t.Fatalf("expected refresh token rotated, got %q", pair2.RefreshToken)
	}

	_, _, err = s.Refresh(ctx, sid, oldRefresh)
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("expected ErrInvalidRefresh for old refresh, got %v", err)
	}

	_, _, err = s.Refresh(ctx, sid, pair2.RefreshToken)
	if err != nil {
		t.Fatalf("expected new refresh to work, got %v", err)
	}
}

func TestService_Refresh_ExpiredSession(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(300, 0)}
	iss := &fakeIssuer{}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a"}}

	u := mustUser(t, "u1", "subj-1", false)
	mustUpsertUser(t, ctx, st, u)
	_ = mustUpsertPasswordCred(t, ctx, st, "cred-1", u.ID(), "pw")

	s := newService(t, st, clk, iss, rbac, false)
	s.cfg.RefreshTTL = 10 * time.Second

	pair, id, err := s.Login(ctx, enum.Password, "subj-1", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	clk.Advance(20 * time.Second)

	_, _, err = s.Refresh(ctx, id.SessionID, pair.RefreshToken)
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("expected ErrInvalidRefresh, got %v", err)
	}
}

func TestService_Refresh_DisabledUser(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(400, 0)}
	iss := &fakeIssuer{}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a"}}

	u := mustUser(t, "u1", "subj-1", false)
	mustUpsertUser(t, ctx, st, u)
	_ = mustUpsertPasswordCred(t, ctx, st, "cred-1", u.ID(), "pw")

	s := newService(t, st, clk, iss, rbac, false)

	pair, id, err := s.Login(ctx, enum.Password, "subj-1", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	u2, err := st.GetUser(ctx, u.ID())
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	u2.Disable()
	if err := st.UpsertUser(ctx, u2); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	_, _, err = s.Refresh(ctx, id.SessionID, pair.RefreshToken)
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("expected ErrInvalidRefresh, got %v", err)
	}
}

func TestService_Revoke(t *testing.T) {
	ctx := context.Background()
	st := inmemory.New()
	clk := &fakeClock{t: time.Unix(500, 0)}
	iss := &fakeIssuer{}
	rbac := &fakeRBAC{perms: []enum.Permission{"perm:a"}}

	u := mustUser(t, "u1", "subj-1", false)
	mustUpsertUser(t, ctx, st, u)
	_ = mustUpsertPasswordCred(t, ctx, st, "cred-1", u.ID(), "pw")

	s := newService(t, st, clk, iss, rbac, false)

	t.Run("empty session id", func(t *testing.T) {
		err := s.Revoke(ctx, "")
		if !errors.Is(err, auth.ErrInvalidRequest) {
			t.Fatalf("expected ErrInvalidRequest, got %v", err)
		}
	})

	t.Run("not found => invalid request", func(t *testing.T) {
		err := s.Revoke(ctx, "nope")
		if !errors.Is(err, auth.ErrInvalidRequest) {
			t.Fatalf("expected ErrInvalidRequest, got %v", err)
		}
	})

	t.Run("success and then refresh rejected as revoked", func(t *testing.T) {
		pair, id, err := s.Login(ctx, enum.Password, "subj-1", "pw")
		if err != nil {
			t.Fatalf("Login: %v", err)
		}

		if err = s.Revoke(ctx, id.SessionID); err != nil {
			t.Fatalf("Revoke: %v", err)
		}

		_, _, err = s.Refresh(ctx, id.SessionID, pair.RefreshToken)
		if !errors.Is(err, auth.ErrRevoked) {
			t.Fatalf("expected ErrRevoked, got %v", err)
		}
	})
}
