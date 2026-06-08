// Package storage defines persistence contracts for control-plane domain entities.
//
// It provides backend-agnostic interfaces describing how domain objects
// (User, Role, Agent, Credential, Session, Verifier) are stored and retrieved.
//
// Design goals
//
//   - Backend independence: domain and application layers depend only on these interfaces, not on concrete implementations.
//   - Explicit error semantics: all methods document which sentinel errors may be returned
//     (ErrNotFound, ErrInvalidArgument, ErrConflict, ErrAlreadyExists, ErrUnavailable, ErrInternal).
//   - Deterministic pagination: all list operations must follow the ordering (CreatedAt DESC, ID ASC)
//     and use opaque cursor-based pagination. CreatedAt (immutable) is the sort key, not UpdatedAt,
//     so a record edited mid-pagination cannot move between pages, which is what guarantees no duplicates or gaps.
//   - Filter isolation: filter types are backend-specific. A backend must validate that a provided filter
//     was constructed for that backend and return ErrInvalidArgument otherwise.
//
// Error model
//   - ErrInvalidArgument - caller error (bad input, malformed cursor, wrong filter).
//   - ErrNotFound - entity does not exist.
//   - ErrAlreadyExists - create conflict.
//   - ErrConflict - concurrent modification or version mismatch.
//   - ErrUnavailable - temporary backend failure (retryable).
//   - ErrInternal - unexpected or invariant-violating storage failure.
package storage
