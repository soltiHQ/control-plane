package domain

import "time"

// Entity defines the contract for mutable domain entities that can be stored in repository-like storage.
//
// Implementations must guarantee:
//   - Stable, unique ID for the lifetime of the entity.
//   - Clone() returns a deep copy (no shared references).
//   - UpdatedAt() advances on every *business* mutation — a domain-meaningful
//     change such as a status/role/secret edit.
//
// Exempt: low-level `Set*` setters that exist only to reconstruct stored state (e.g. SetCreatedAt/SetUpdatedAt/SetLastSeenAt).
// They restore the persisted value verbatim and MUST NOT advance UpdatedAt — otherwise loading an entity
// from storage or replaying it from Raft would overwrite its real timestamp and
// look like a fresh change.
type Entity[T any] interface {
	// ID returns the unique identifier for this entity.
	ID() string
	// Clone creates a deep copy of the entity.
	Clone() T
	// CreatedAt returns the creation timestamp.
	CreatedAt() time.Time
	// UpdatedAt returns the last modification timestamp.
	UpdatedAt() time.Time
}
