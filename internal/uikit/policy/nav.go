package policy

import "github.com/soltiHQ/control-plane/internal/auth/identity"

// Nav is a UI-oriented navigation model derived from the current identity.
//
// It encapsulates which top-level menu items the authenticated user may see
// (Show* flags) and which create-actions are available (Can* flags).
type Nav struct {
	ShowUsers  bool
	ShowTasks  bool
	ShowAgents bool
	CanAddUser bool
	CanAddSpec bool
}

// BuildNav derives UI navigation flags from the authenticated identity.
func BuildNav(id *identity.Identity) Nav {
	if id == nil {
		return Nav{}
	}

	perms := permSet(id)
	return Nav{
		CanAddUser: hasAny(perms, usersAdd),
		ShowTasks:  hasAny(perms, specsGet),
		CanAddSpec: hasAny(perms, specsAdd),
		ShowAgents: hasAny(perms, agentsGet, agentsEdit),
		ShowUsers:  hasAny(perms, usersGet, usersAdd, usersEdit, usersDelete),
	}
}
