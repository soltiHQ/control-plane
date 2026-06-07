package policy

import (
	"testing"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/internal/auth/identity"
)

func id(userID string, perms ...enum.Permission) *identity.Identity {
	return &identity.Identity{UserID: userID, Permissions: perms}
}

func TestBuildNav(t *testing.T) {
	t.Run("nil identity is all-false", func(t *testing.T) {
		if got := BuildNav(nil); got != (Nav{}) {
			t.Fatalf("BuildNav(nil) = %+v, want zero", got)
		}
	})

	t.Run("any users permission shows the users menu", func(t *testing.T) {
		if got := BuildNav(id("u1", usersDelete)); !got.ShowUsers {
			t.Fatalf("ShowUsers = false, want true (usersDelete should suffice)")
		}
	})

	t.Run("full flag mapping", func(t *testing.T) {
		got := BuildNav(id("u1", usersAdd, specsGet, specsAdd, agentsGet))
		want := Nav{
			ShowUsers:  true, // usersAdd
			ShowTasks:  true, // specsGet
			ShowAgents: true, // agentsGet
			CanAddUser: true, // usersAdd
			CanAddSpec: true, // specsAdd
		}
		if got != want {
			t.Fatalf("BuildNav = %+v, want %+v", got, want)
		}
	})

	t.Run("no permissions is all-false", func(t *testing.T) {
		if got := BuildNav(id("u1")); got != (Nav{}) {
			t.Fatalf("BuildNav(no perms) = %+v, want zero", got)
		}
	})
}

func TestBuildUserDetail(t *testing.T) {
	t.Run("nil identity is all-false", func(t *testing.T) {
		if got := BuildUserDetail(nil, "target"); got != (UserDetail{}) {
			t.Fatalf("BuildUserDetail(nil) = %+v, want zero", got)
		}
	})

	t.Run("other user: edit/roles/delete follow permissions", func(t *testing.T) {
		got := BuildUserDetail(id("admin", usersEdit, usersDelete), "victim")
		want := UserDetail{IsSelf: false, CanEdit: true, CanEditRoles: true, CanDelete: true}
		if got != want {
			t.Fatalf("BuildUserDetail(other) = %+v, want %+v", got, want)
		}
	})

	t.Run("self-guard: cannot delete or change own roles even with permission", func(t *testing.T) {
		got := BuildUserDetail(id("me", usersEdit, usersDelete), "me")
		want := UserDetail{
			IsSelf:       true,
			CanEdit:      true,  // editing own profile is allowed
			CanEditRoles: false, // forced off when self
			CanDelete:    false, // forced off when self
		}
		if got != want {
			t.Fatalf("BuildUserDetail(self) = %+v, want %+v", got, want)
		}
	})
}

func TestBuildAgentDetail(t *testing.T) {
	if got := BuildAgentDetail(nil); got != (AgentDetail{}) {
		t.Fatalf("BuildAgentDetail(nil) = %+v, want zero", got)
	}
	if got := BuildAgentDetail(id("u1", agentsEdit)); !got.CanEditLabels {
		t.Fatalf("CanEditLabels = false, want true")
	}
	if got := BuildAgentDetail(id("u1", agentsGet)); got.CanEditLabels {
		t.Fatalf("CanEditLabels = true with only agentsGet, want false")
	}
}

func TestBuildSpecDetail(t *testing.T) {
	if got := BuildSpecDetail(nil); got != (SpecDetail{}) {
		t.Fatalf("BuildSpecDetail(nil) = %+v, want zero", got)
	}

	got := BuildSpecDetail(id("u1", specsEdit))
	if !got.CanEdit || !got.CanDelete {
		t.Fatalf("specsEdit should grant CanEdit and CanDelete, got %+v", got)
	}
	if got.CanDeploy {
		t.Fatalf("CanDeploy = true without specsDeploy, want false")
	}

	if got = BuildSpecDetail(id("u1", specsDeploy)); !got.CanDeploy {
		t.Fatalf("specsDeploy should grant CanDeploy, got %+v", got)
	}
}
