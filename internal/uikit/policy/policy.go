package policy

import (
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/internal/auth/identity"
)

// Convenience aliases to keep policy builders readable.
const (
	// agents
	agentsGet  = enum.AgentsGet
	agentsEdit = enum.AgentsEdit

	// users
	usersGet    = enum.UsersGet
	usersAdd    = enum.UsersAdd
	usersEdit   = enum.UsersEdit
	usersDelete = enum.UsersDelete

	// specs (task specs)
	specsGet    = enum.SpecsGet
	specsAdd    = enum.SpecsAdd
	specsEdit   = enum.SpecsEdit
	specsDeploy = enum.SpecsDeploy
)

func permSet(id *identity.Identity) map[enum.Permission]struct{} {
	m := make(map[enum.Permission]struct{}, len(id.Permissions))
	for _, p := range id.Permissions {
		m[p] = struct{}{}
	}
	return m
}

func hasAny(set map[enum.Permission]struct{}, wants ...enum.Permission) bool {
	for _, w := range wants {
		if _, ok := set[w]; ok {
			return true
		}
	}
	return false
}
