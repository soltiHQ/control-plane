package model

import (
	"time"

	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/enum"
)

var _ domain.Entity[*Role] = (*Role)(nil)

// Role represents a named collection of permissions.
type Role struct {
	createdAt time.Time
	updatedAt time.Time

	id   string
	name string

	permissions []enum.Permission
}

// NewRole creates a new role entity.
func NewRole(id, name string) (*Role, error) {
	if id == "" {
		return nil, domain.ErrEmptyID
	}
	if name == "" {
		return nil, domain.ErrEmptyName
	}

	now := time.Now()
	return &Role{
		createdAt:   now,
		updatedAt:   now,
		id:          id,
		name:        name,
		permissions: make([]enum.Permission, 0),
	}, nil
}

// ID returns the unique identifier of the role.
func (r *Role) ID() string { return r.id }

// Name returns the role's name.
func (r *Role) Name() string { return r.name }

// CreatedAt returns the timestamp when the role was created.
func (r *Role) CreatedAt() time.Time { return r.createdAt }

// UpdatedAt returns the timestamp of the last modification.
func (r *Role) UpdatedAt() time.Time { return r.updatedAt }

// SetCreatedAt restores the creation timestamp (persistence hook).
func (r *Role) SetCreatedAt(t time.Time) { r.createdAt = t }

// SetUpdatedAt restores the modification timestamp (persistence hook).
func (r *Role) SetUpdatedAt(t time.Time) { r.updatedAt = t }

// PermissionsAll returns a copy of all permissions assigned to the role.
func (r *Role) PermissionsAll() []enum.Permission {
	out := make([]enum.Permission, len(r.permissions))
	copy(out, r.permissions)
	return out
}

// PermissionHas returns true if the role contains the given permission.
func (r *Role) PermissionHas(p enum.Permission) bool {
	for _, x := range r.permissions {
		if x == p {
			return true
		}
	}
	return false
}

// PermissionAdd grants a permission to the role.
func (r *Role) PermissionAdd(p enum.Permission) error {
	if p == "" {
		return domain.ErrFieldEmpty
	}
	if r.PermissionHas(p) {
		return nil
	}

	r.permissions = append(r.permissions, p)
	r.updatedAt = time.Now()
	return nil
}

// PermissionDelete revokes a permission from the role.
func (r *Role) PermissionDelete(p enum.Permission) {
	for i, x := range r.permissions {
		if x == p {
			r.permissions = append(r.permissions[:i], r.permissions[i+1:]...)
			r.updatedAt = time.Now()
			return
		}
	}
}

// Rename changes the role name.
func (r *Role) Rename(name string) error {
	if name == "" {
		return domain.ErrEmptyName
	}
	if r.name == name {
		return nil
	}

	r.name = name
	r.updatedAt = time.Now()
	return nil
}

// Clone creates a deep copy of the role entity.
func (r *Role) Clone() *Role {
	out := make([]enum.Permission, len(r.permissions))
	copy(out, r.permissions)

	return &Role{
		createdAt:   r.createdAt,
		updatedAt:   r.updatedAt,
		id:          r.id,
		name:        r.name,
		permissions: out,
	}
}
