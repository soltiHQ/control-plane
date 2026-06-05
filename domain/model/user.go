package model

import (
	"time"

	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/enum"
)

var _ domain.Entity[*User] = (*User)(nil)

// User is a core domain entity representing a system user.
//
// A user can be a human or a service identity. The user is identified by:
//   - id: internal immutable identifier
//   - subject: stable authentication subject (e.g. JWT "sub") used for login mapping
//
// Access is granted via:
//   - role IDs (RBAC roles)
//   - direct per-user permissions (exceptions/overrides)
//
// Notes:
//   - roleIDs and permissions are unique sets (no duplicates).
//   - This model stores assignments; role expansion into effective permissions belongs elsewhere.
type User struct {
	createdAt time.Time
	updatedAt time.Time

	id      string
	subject string
	email   string
	name    string

	roleIDs     []string
	permissions []enum.Permission

	disabled bool
}

// NewUser creates a new user domain entity.
func NewUser(id, subject string) (*User, error) {
	if id == "" {
		return nil, domain.ErrEmptyID
	}
	if subject == "" {
		return nil, domain.ErrInvalidSubject
	}

	now := time.Now()
	return &User{
		createdAt:   now,
		updatedAt:   now,
		id:          id,
		subject:     subject,
		roleIDs:     make([]string, 0),
		permissions: make([]enum.Permission, 0),
	}, nil
}

// ID returns the unique internal identifier of the user.
func (u *User) ID() string { return u.id }

// Subject returns the stable authentication subject (e.g. JWT "sub").
// It is used to map an external identity to this user.
func (u *User) Subject() string { return u.subject }

// Email returns the user's email address (maybe empty if not set).
func (u *User) Email() string { return u.email }

// Name returns the user's display name (maybe empty if not set).
func (u *User) Name() string { return u.name }

// Disabled reports whether the user is disabled.
func (u *User) Disabled() bool { return u.disabled }

// CreatedAt returns the timestamp when the user entity was created.
func (u *User) CreatedAt() time.Time { return u.createdAt }

// UpdatedAt returns the timestamp of the last modification.
func (u *User) UpdatedAt() time.Time { return u.updatedAt }

// SetCreatedAt / SetUpdatedAt — used by persistence adapters.
func (u *User) SetCreatedAt(t time.Time) { u.createdAt = t }
func (u *User) SetUpdatedAt(t time.Time) { u.updatedAt = t }

// EmailAdd updates the user's email.
func (u *User) EmailAdd(email string) {
	if u.email == email {
		return
	}
	u.email = email
	u.updatedAt = time.Now()
}

// NameAdd updates the user's display name.
func (u *User) NameAdd(name string) {
	if u.name == name {
		return
	}
	u.name = name
	u.updatedAt = time.Now()
}

// SubjectAdd updates the user's subject.
func (u *User) SubjectAdd(subject string) {
	if u.subject == subject {
		return
	}
	u.subject = subject
	u.updatedAt = time.Now()
}

// Disable marks the user as disabled.
func (u *User) Disable() {
	if u.disabled {
		return
	}
	u.disabled = true
	u.updatedAt = time.Now()
}

// Enable marks the user as enabled.
func (u *User) Enable() {
	if !u.disabled {
		return
	}
	u.disabled = false
	u.updatedAt = time.Now()
}

// RoleIDsAll returns a copy of role IDs assigned to the user.
func (u *User) RoleIDsAll() []string {
	out := make([]string, len(u.roleIDs))
	copy(out, u.roleIDs)
	return out
}

// RolesIDsNew replaces the user's full set of role IDs.
func (u *User) RolesIDsNew(roles []string) {
	u.updatedAt = time.Now()

	if len(roles) == 0 {
		u.roleIDs = nil
		return
	}

	var (
		out  = make([]string, 0, len(roles))
		seen = make(map[string]struct{}, len(roles))
	)
	for _, id := range roles {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	u.roleIDs = out
}

// PermissionsAll returns a copy of permissions granted directly to the user.
func (u *User) PermissionsAll() []enum.Permission {
	out := make([]enum.Permission, len(u.permissions))
	copy(out, u.permissions)
	return out
}

// PermissionsNew update full list of user's permissions.
func (u *User) PermissionsNew(perms []string) {
	u.updatedAt = time.Now()

	if len(perms) == 0 {
		u.permissions = nil
		return
	}

	var (
		out  = make([]enum.Permission, 0, len(perms))
		seen = make(map[enum.Permission]struct{}, len(perms))
	)
	for _, p := range perms {
		if p == "" {
			continue
		}

		perm := enum.Permission(p)
		if _, ok := seen[perm]; ok {
			continue
		}
		seen[perm] = struct{}{}
		out = append(out, perm)
	}
	u.permissions = out
}

// RoleHas reports whether the user has the given role ID assigned.
func (u *User) RoleHas(roleID string) bool {
	for _, id := range u.roleIDs {
		if id == roleID {
			return true
		}
	}
	return false
}

// PermissionHas reports whether the user has the given permission granted directly.
func (u *User) PermissionHas(p enum.Permission) bool {
	for _, x := range u.permissions {
		if x == p {
			return true
		}
	}
	return false
}

// RoleAdd assigns a role to the user.
// It is idempotent: adding an existing role does nothing.
func (u *User) RoleAdd(roleID string) error {
	if roleID == "" {
		return domain.ErrEmptyID
	}
	if u.RoleHas(roleID) {
		return nil
	}
	u.roleIDs = append(u.roleIDs, roleID)
	u.updatedAt = time.Now()
	return nil
}

// RoleDelete removes a role from the user.
// If a role does not exist, nothing happens.
func (u *User) RoleDelete(roleID string) {
	for i, id := range u.roleIDs {
		if id == roleID {
			u.roleIDs = append(u.roleIDs[:i], u.roleIDs[i+1:]...)
			u.updatedAt = time.Now()
			return
		}
	}
}

// PermissionAdd grants a permission directly to the user.
// It is idempotent: adding an existing permission does nothing.
func (u *User) PermissionAdd(p enum.Permission) error {
	if p == "" {
		return domain.ErrFieldEmpty
	}
	if u.PermissionHas(p) {
		return nil
	}
	u.permissions = append(u.permissions, p)
	u.updatedAt = time.Now()
	return nil
}

// PermissionDelete revokes a permission from the user.
// If permission does not exist, nothing happens.
func (u *User) PermissionDelete(p enum.Permission) {
	for i, x := range u.permissions {
		if x == p {
			u.permissions = append(u.permissions[:i], u.permissions[i+1:]...)
			u.updatedAt = time.Now()
			return
		}
	}
}

// Clone creates a deep copy of the user entity.
func (u *User) Clone() *User {
	var (
		roleIDs = make([]string, len(u.roleIDs))
		perms   = make([]enum.Permission, len(u.permissions))
	)

	copy(roleIDs, u.roleIDs)
	copy(perms, u.permissions)
	return &User{
		createdAt:   u.createdAt,
		updatedAt:   u.updatedAt,
		id:          u.id,
		subject:     u.subject,
		email:       u.email,
		name:        u.name,
		roleIDs:     roleIDs,
		permissions: perms,
		disabled:    u.disabled,
	}
}
