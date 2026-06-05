package inmemory

import (
	"testing"
	"time"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
)

func fixedNow() time.Time {
	return time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC)
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func requireNotNil[T any](t *testing.T, v *T) {
	t.Helper()
	if v == nil {
		t.Fatalf("unexpected nil")
	}
}

func mkAgent(t *testing.T, id string) *model.Agent {
	t.Helper()
	a, err := model.NewAgent(id, "agent-"+id, "http://"+id)
	requireNoErr(t, err)
	requireNotNil(t, a)
	return a
}

func mkUser(t *testing.T, id, subject string) *model.User {
	t.Helper()
	u, err := model.NewUser(id, subject)
	requireNoErr(t, err)
	requireNotNil(t, u)

	u.SetEmail(subject + "@example.com")
	u.SetName("User " + id)

	return u
}

func mkRole(t *testing.T, id, name string) *model.Role {
	t.Helper()
	r, err := model.NewRole(id, name)
	requireNoErr(t, err)
	requireNotNil(t, r)
	return r
}

func mkCredential(t *testing.T, id, userID string, auth enum.Auth) *model.Credential {
	t.Helper()
	c, err := model.NewCredential(id, userID, auth)
	requireNoErr(t, err)
	requireNotNil(t, c)
	return c
}

func mkVerifier(t *testing.T, id, credentialID string, auth enum.Auth) *model.Verifier {
	t.Helper()
	v, err := model.NewVerifier(id, credentialID, auth)
	requireNoErr(t, err)
	requireNotNil(t, v)
	return v
}

func mkSession(t *testing.T, id, userID, credentialID string, auth enum.Auth) *model.Session {
	t.Helper()
	refreshHash := []byte("refresh-hash-" + id)
	expiresAt := fixedNow().Add(24 * time.Hour)

	s, err := model.NewSession(id, userID, credentialID, auth, refreshHash, expiresAt)
	requireNoErr(t, err)
	requireNotNil(t, s)
	return s
}

func userAddRole(t *testing.T, u *model.User, roleID string) {
	t.Helper()
	requireNoErr(t, u.RoleAdd(roleID))
}

func userAddPerm(t *testing.T, u *model.User, p enum.Permission) {
	t.Helper()
	requireNoErr(t, u.PermissionAdd(p))
}

func roleAddPerm(t *testing.T, r *model.Role, p enum.Permission) {
	t.Helper()
	requireNoErr(t, r.PermissionAdd(p))
}
