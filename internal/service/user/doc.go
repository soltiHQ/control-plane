// Package user implements user management use-cases:
//   - Paginated listing and retrieval (by ID or subject)
//   - Upsert with field normalization and uniqueness checks
//   - Cascading deletion (sessions → verifiers → credentials → user).
package user
