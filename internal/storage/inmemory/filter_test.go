package inmemory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/internal/storage"
)

func TestAgentFilter_Matches_AllPredicatesANDed(t *testing.T) {
	t.Parallel()

	a := mkAgent(t, "a1")
	a.LabelAdd("env", "prod")
	a.LabelAdd("tier", "edge")

	if !NewAgentFilter().Matches(a) {
		t.Fatalf("empty filter must match")
	}

	if !NewAgentFilter().ByLabel("env", "prod").Matches(a) {
		t.Fatalf("expected match by label")
	}
	if NewAgentFilter().ByLabel("env", "dev").Matches(a) {
		t.Fatalf("expected no match by label value")
	}
	if NewAgentFilter().ByLabel("missing", "x").Matches(a) {
		t.Fatalf("expected no match for missing label")
	}

	f := NewAgentFilter().
		ByLabel("env", "prod").
		ByLabel("tier", "edge")
	if !f.Matches(a) {
		t.Fatalf("expected match for combined labels")
	}

	f2 := NewAgentFilter().
		ByLabel("env", "prod").
		ByLabel("tier", "core")
	if f2.Matches(a) {
		t.Fatalf("expected no match for ANDed predicates")
	}
}

func TestAgentFilter_StaleAtBefore(t *testing.T) {
	t.Parallel()

	now := time.Now()
	a := mkAgent(t, "a1")

	// Zero staleAt: always matches (safe default).
	if !NewAgentFilter().StaleAtBefore(now).Matches(a) {
		t.Fatalf("zero staleAt must match")
	}

	// staleAt in the past: matches.
	a.SetStaleAt(now.Add(-time.Minute))
	if !NewAgentFilter().StaleAtBefore(now).Matches(a) {
		t.Fatalf("past staleAt must match")
	}

	// staleAt in the future: does not match.
	a.SetStaleAt(now.Add(time.Minute))
	if NewAgentFilter().StaleAtBefore(now).Matches(a) {
		t.Fatalf("future staleAt must not match")
	}

	// Combined with another predicate.
	a.SetStaleAt(now.Add(-time.Minute))
	a.LabelAdd("env", "prod")
	f := NewAgentFilter().StaleAtBefore(now).ByLabel("env", "prod")
	if !f.Matches(a) {
		t.Fatalf("combined filter must match")
	}
}

func TestUserFilter_Matches_AllPredicatesANDed(t *testing.T) {
	t.Parallel()

	u := mkUser(t, "u1", "sub-1")
	u.SetEmail("sub-1@example.com")
	u.SetName("User 1")
	userAddRole(t, u, "r-admin")
	userAddPerm(t, u, enum.UsersGet)

	if !NewUserFilter().Matches(u) {
		t.Fatalf("empty filter must match")
	}

	if !NewUserFilter().ByEmail("sub-1@example.com").Matches(u) {
		t.Fatalf("expected match by email")
	}
	if NewUserFilter().ByEmail("x@example.com").Matches(u) {
		t.Fatalf("expected no match by email")
	}

	if !NewUserFilter().ByDisabled(false).Matches(u) {
		t.Fatalf("expected match by disabled=false")
	}
	u.Disable()
	if !NewUserFilter().ByDisabled(true).Matches(u) {
		t.Fatalf("expected match by disabled=true")
	}

	if !NewUserFilter().ByRoleID("r-admin").Matches(u) {
		t.Fatalf("expected match by role id")
	}
	if NewUserFilter().ByRoleID("r-viewer").Matches(u) {
		t.Fatalf("expected no match by role id")
	}

	if !NewUserFilter().ByPermission(enum.UsersGet).Matches(u) {
		t.Fatalf("expected match by permission")
	}
	if NewUserFilter().ByPermission(enum.UsersDelete).Matches(u) {
		t.Fatalf("expected no match by permission")
	}

	f := NewUserFilter().
		ByDisabled(true).
		ByEmail("sub-1@example.com").
		ByRoleID("r-admin").
		ByPermission(enum.UsersGet)
	if !f.Matches(u) {
		t.Fatalf("expected match for ANDed predicates")
	}

	f2 := NewUserFilter().
		ByDisabled(true).
		ByEmail("sub-1@example.com").
		ByRoleID("r-admin").
		ByPermission(enum.UsersDelete)
	if f2.Matches(u) {
		t.Fatalf("expected no match for ANDed predicates")
	}
}

func TestRoleFilter_Matches_AllPredicatesANDed(t *testing.T) {
	t.Parallel()

	r := mkRole(t, "r1", "admin")
	roleAddPerm(t, r, enum.UsersGet)
	roleAddPerm(t, r, enum.AgentsEdit)

	if !NewRoleFilter().Matches(r) {
		t.Fatalf("empty filter must match")
	}

	if !NewRoleFilter().ByName("admin").Matches(r) {
		t.Fatalf("expected match by name")
	}
	if NewRoleFilter().ByName("viewer").Matches(r) {
		t.Fatalf("expected no match by name")
	}

	if !NewRoleFilter().ByPermission(enum.UsersGet).Matches(r) {
		t.Fatalf("expected match by permission")
	}
	if NewRoleFilter().ByPermission(enum.UsersDelete).Matches(r) {
		t.Fatalf("expected no match by permission")
	}

	f := NewRoleFilter().
		ByName("admin").
		ByPermission(enum.AgentsEdit)
	if !f.Matches(r) {
		t.Fatalf("expected match for ANDed predicates")
	}

	f2 := NewRoleFilter().
		ByName("admin").
		ByPermission(enum.UsersDelete)
	if f2.Matches(r) {
		t.Fatalf("expected no match for ANDed predicates")
	}
}

func TestStore_List_FilterTypes_AreBackendSpecific(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := New()

	requireNoErr(t, s.UpsertAgent(ctx, mkAgent(t, "a1")))
	requireNoErr(t, s.UpsertUser(ctx, mkUser(t, "u1", "sub-1")))
	requireNoErr(t, s.UpsertRole(ctx, mkRole(t, "r1", "admin")))

	if _, err := s.ListAgents(ctx, NewAgentFilter(), storage.ListOptions{}); err != nil {
		t.Fatalf("expected no error, err=%v", err)
	}
	if _, err := s.ListUsers(ctx, NewUserFilter(), storage.ListOptions{}); err != nil {
		t.Fatalf("expected no error, err=%v", err)
	}
	if _, err := s.ListRoles(ctx, NewRoleFilter(), storage.ListOptions{}); err != nil {
		t.Fatalf("expected no error, err=%v", err)
	}

	type foreignAgentFilter struct{}
	type foreignUserFilter struct{}
	type foreignRoleFilter struct{}

	var af storage.AgentFilter = (*foreignAgentFilter)(nil)
	var uf storage.UserFilter = (*foreignUserFilter)(nil)
	var rf storage.RoleFilter = (*foreignRoleFilter)(nil)

	_, err := s.ListAgents(ctx, af, storage.ListOptions{})
	if !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, err=%v", err)
	}

	_, err = s.ListUsers(ctx, uf, storage.ListOptions{})
	if !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, err=%v", err)
	}

	_, err = s.ListRoles(ctx, rf, storage.ListOptions{})
	if !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, err=%v", err)
	}
}
