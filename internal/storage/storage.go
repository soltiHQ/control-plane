package storage

import (
	"context"
	"time"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
)

// AgentListResult contains a page of agent results with pagination support.
type AgentListResult = ListResult[*model.Agent]

// UserListResult contains a page of user results with pagination support.
type UserListResult = ListResult[*model.User]

// CredentialListResult contains a page of credential results with pagination support.
type CredentialListResult = ListResult[*model.Credential]

// RoleListResult contains a page of role results with pagination support.
type RoleListResult = ListResult[*model.Role]

// VerifierListResult contains a page of verifier results with pagination support.
type VerifierListResult = ListResult[*model.Verifier]

// SessionListResult contains a page of session results with pagination support.
type SessionListResult = ListResult[*model.Session]

// SpecListResult contains a page of task spec results with pagination support.
type SpecListResult = ListResult[*model.Spec]

// RolloutListResult contains a page of rollout results with pagination support.
type RolloutListResult = ListResult[*model.Rollout]

// AgentStore defines persistence operations for agent entities.
type AgentStore interface {
	// UpsertAgent creates a new agent or replaces an existing one.
	//
	// If an agent with the same ID exists, it is fully replaced.
	// Otherwise, a new agent record is created.
	//
	// Returns:
	//   - ErrInvalidArgument if the agent violates storage-level invariants.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	UpsertAgent(ctx context.Context, a *model.Agent) error

	// GetAgent retrieves an agent by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no agent with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetAgent(ctx context.Context, id string) (*model.Agent, error)

	// ListAgents retrieves agents matching the provided filter with pagination support.
	//
	// Ordering and cursor contract are defined by ListOptions.
	//
	// Returns:
	//   - ErrInvalidArgument if the filter type is incompatible or the cursor is malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	ListAgents(ctx context.Context, filter AgentFilter, opts ListOptions) (*AgentListResult, error)

	// DeleteAgent removes an agent by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no agent with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteAgent(ctx context.Context, id string) error
}

// AgentCredentialStore persists per-agent bearer secrets learned at discovery
// (trust-on-first-use). Keyed by agent ID — one credential per agent. The token
// is raw (the control plane must present it on outbound calls), so it is never
// surfaced to API/UI and never logged in clear.
type AgentCredentialStore interface {
	// UpsertAgentCredential stores or replaces an agent's credential.
	//
	// Returns:
	//   - ErrInvalidArgument if the credential is nil or violates storage-level invariants.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	UpsertAgentCredential(ctx context.Context, c *model.AgentCredential) error

	// GetAgentCredential retrieves the credential bound to an agent.
	//
	// Returns:
	//   - ErrNotFound if the agent has no credential yet (not enrolled).
	//   - ErrInvalidArgument if the agent ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetAgentCredential(ctx context.Context, agentID string) (*model.AgentCredential, error)

	// DeleteAgentCredential removes an agent's credential (revoke / reset).
	//
	// Returns:
	//   - ErrNotFound if no credential exists for the agent.
	//   - ErrInvalidArgument if the agent ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteAgentCredential(ctx context.Context, agentID string) error
}

// UserStore defines persistence operations for user entities.
type UserStore interface {
	// UpsertUser creates a new user or replaces an existing one.
	//
	// Returns:
	//   - ErrInvalidArgument if the user is nil or violates storage-level invariants.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	UpsertUser(ctx context.Context, u *model.User) error

	// GetUser retrieves a user by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no user with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetUser(ctx context.Context, id string) (*model.User, error)

	// GetUserBySubject retrieves a user by their subject identifier (e.g., JWT "sub").
	//
	// Returns:
	//   - ErrNotFound if no user with the given subject exists.
	//   - ErrInvalidArgument if the subject is empty.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetUserBySubject(ctx context.Context, subject string) (*model.User, error)

	// ListUsers retrieves users matching the provided filter with pagination support.
	//
	// Ordering and cursor contract are defined by ListOptions.
	//
	// Returns:
	//   - ErrInvalidArgument if the filter type is incompatible or the cursor is malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	ListUsers(ctx context.Context, filter UserFilter, opts ListOptions) (*UserListResult, error)

	// DeleteUser removes a user by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no user with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteUser(ctx context.Context, id string) error
}

// CredentialStore defines persistence operations for credential entities.
type CredentialStore interface {
	// UpsertCredential creates a new credential or replaces an existing one.
	//
	// Returns:
	//   - ErrInvalidArgument if the credential is nil or violates storage-level invariants.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	UpsertCredential(ctx context.Context, c *model.Credential) error

	// GetCredential retrieves a credential by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no credential with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetCredential(ctx context.Context, id string) (*model.Credential, error)

	// GetCredentialByUserAndAuth retrieves a specific auth kind credential for a user.
	//
	// Returns:
	//   - ErrNotFound if no matching credential exists.
	//   - ErrInvalidArgument if userID is empty.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetCredentialByUserAndAuth(ctx context.Context, userID string, auth enum.Auth) (*model.Credential, error)

	// ListCredentialsByUser retrieves all credentials for a specific user.
	//
	// Returns:
	//   - ErrInvalidArgument if userID is empty.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	ListCredentialsByUser(ctx context.Context, userID string) ([]*model.Credential, error)

	// DeleteCredential removes a credential by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no credential with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteCredential(ctx context.Context, id string) error
}

// VerifierStore defines persistence operations for verifier entities.
type VerifierStore interface {
	// UpsertVerifier creates a new verifier or replaces an existing one.
	//
	// Returns:
	//   - ErrInvalidArgument if the verifier is nil or violates storage-level invariants.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	UpsertVerifier(ctx context.Context, v *model.Verifier) error

	// GetVerifier retrieves a verifier by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no verifier with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetVerifier(ctx context.Context, id string) (*model.Verifier, error)

	// GetVerifierByCredential retrieves verifier for a given credential.
	//
	// Returns:
	//   - ErrNotFound if no verifier for the given credential exists.
	//   - ErrInvalidArgument if credentialID is empty.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetVerifierByCredential(ctx context.Context, credentialID string) (*model.Verifier, error)

	// DeleteVerifierByCredential removes verifier for a given credential.
	//
	// Semantics:
	//   - Idempotent: deleting a missing verifier is a no-op.
	//
	// Returns:
	//   - ErrInvalidArgument if credentialID is empty.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteVerifierByCredential(ctx context.Context, credentialID string) error

	// DeleteVerifier removes a verifier by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no verifier with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteVerifier(ctx context.Context, id string) error
}

// SessionStore defines persistence operations for session entities.
type SessionStore interface {
	// CreateSession creates a new session.
	//
	// Returns:
	//   - ErrInvalidArgument if the session is nil or has an empty ID.
	//   - ErrAlreadyExists if a session with the same ID already exists.
	//   - ErrInternal for unexpected storage failures.
	CreateSession(ctx context.Context, s *model.Session) error

	// GetSession retrieves a session by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no session with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty.
	//   - ErrInternal for unexpected storage failures.
	GetSession(ctx context.Context, id string) (*model.Session, error)

	// ListSessionsByUser retrieves all sessions for a specific user.
	//
	// Returns:
	//   - ErrInvalidArgument if userID is empty.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	ListSessionsByUser(ctx context.Context, userID string) ([]*model.Session, error)

	// RotateRefresh updates the refresh token hash and expiry for the given session.
	//
	// This is used for refresh token rotation.
	//
	// Returns:
	//   - ErrNotFound if no session with the given ID exists.
	//   - ErrInvalidArgument if arguments are invalid.
	//   - ErrInternal for unexpected storage failures.
	RotateRefresh(ctx context.Context, sessionID string, newHash []byte, newExpiresAt time.Time) error

	// RevokeSession marks a session as revoked.
	//
	// Returns:
	//   - ErrNotFound if no session with the given ID exists.
	//   - ErrInvalidArgument if arguments are invalid.
	//   - ErrInternal for unexpected storage failures.
	RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error

	// DeleteSession removes a session by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no session with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty.
	//   - ErrInternal for unexpected storage failures.
	DeleteSession(ctx context.Context, id string) error

	// DeleteSessionsByUser removes all sessions for a user.
	//
	// Idempotent: if the user has no sessions, the operation is a no-op.
	//
	// Returns:
	//   - ErrInvalidArgument if userID is empty.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteSessionsByUser(ctx context.Context, userID string) error
}

// RoleStore defines persistence operations for role entities.
type RoleStore interface {
	// UpsertRole creates a new role or replaces an existing one.
	//
	// Returns:
	//   - ErrInvalidArgument if the role is nil or violates storage-level invariants.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	UpsertRole(ctx context.Context, r *model.Role) error

	// GetRole retrieves a role by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no role with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetRole(ctx context.Context, id string) (*model.Role, error)

	// GetRoles retrieves roles by their IDs.
	//
	// Returns:
	//   - ErrInvalidArgument if ids are empty or contain empty elements.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetRoles(ctx context.Context, ids []string) ([]*model.Role, error)

	// GetRoleByName retrieves a role by its name.
	//
	// Returns:
	//   - ErrNotFound if no role with the given name exists.
	//   - ErrInvalidArgument if the name is empty.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetRoleByName(ctx context.Context, name string) (*model.Role, error)

	// ListRoles retrieves roles matching the provided filter with pagination support.
	//
	// Ordering and cursor contract are defined by ListOptions.
	//
	// Returns:
	//   - ErrInvalidArgument if the filter type is incompatible or the cursor is malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	ListRoles(ctx context.Context, filter RoleFilter, opts ListOptions) (*RoleListResult, error)

	// DeleteRole removes a role by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no role with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteRole(ctx context.Context, id string) error
}

// SpecStore defines persistence operations for spec entities.
type SpecStore interface {
	// UpsertSpec creates a new spec or replaces an existing one.
	//
	// Returns:
	//   - ErrInvalidArgument if the spec is nil or violates storage-level invariants.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	UpsertSpec(ctx context.Context, ts *model.Spec) error

	// GetSpec retrieves a spec by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no spec with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetSpec(ctx context.Context, id string) (*model.Spec, error)

	// ListSpecs retrieves specs matching the provided filter with pagination support.
	//
	// Ordering and cursor contract are defined by ListOptions.
	//
	// Returns:
	//   - ErrInvalidArgument if the filter type is incompatible or the cursor is malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	ListSpecs(ctx context.Context, filter SpecFilter, opts ListOptions) (*SpecListResult, error)

	// DeleteSpec removes a spec by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no spec with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteSpec(ctx context.Context, id string) error
}

// RolloutStore defines persistence operations for rollout entities.
type RolloutStore interface {
	// UpsertRollout creates a new rollout or replaces an existing one.
	//
	// Returns:
	//   - ErrInvalidArgument if the rollout is nil or violates storage-level invariants.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	UpsertRollout(ctx context.Context, ss *model.Rollout) error

	// GetRollout retrieves a rollout by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no rollout with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	GetRollout(ctx context.Context, id string) (*model.Rollout, error)

	// ListRollouts retrieves rollouts matching the provided filter with pagination support.
	//
	// Ordering and cursor contract are defined by ListOptions.
	//
	// Returns:
	//   - ErrInvalidArgument if the filter type is incompatible or the cursor is malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	ListRollouts(ctx context.Context, filter RolloutFilter, opts ListOptions) (*RolloutListResult, error)

	// DeleteRollout removes a rollout by its unique identifier.
	//
	// Returns:
	//   - ErrNotFound if no rollout with the given ID exists.
	//   - ErrInvalidArgument if the ID is empty or malformed.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteRollout(ctx context.Context, id string) error

	// DeleteRolloutsBySpec removes all rollouts associated with a given spec.
	//
	// Idempotent: if no rollouts exist for the spec, the operation is a no-op.
	//
	// Returns:
	//   - ErrInvalidArgument if specID is empty.
	//   - ErrUnavailable if the backend is temporarily unavailable.
	//   - ErrInternal for unexpected storage failures.
	DeleteRolloutsBySpec(ctx context.Context, specID string) error
}

// FilterFactory defines constructors for backend-specific filters built from backend-agnostic query criteria.
type FilterFactory interface {
	// BuildRolloutFilter constructs a RolloutFilter for this backend from the supplied criteria.
	// Empty criteria match every rollout.
	BuildRolloutFilter(c RolloutQueryCriteria) RolloutFilter

	// BuildSpecFilter constructs a SpecFilter for this backend from the supplied criteria.
	// Empty criteria match every spec.
	BuildSpecFilter(c SpecQueryCriteria) SpecFilter
}

// TxStore groups transactional operations.
//
// WithTx runs fn inside a single atomic transaction: mutations become visible after fn returns nil;
// on error the transaction is rolled back and no changes are observed.
//
// Contract: fn must contain only calls to tx.
// No network I/O, no event publication, no stateful side-effects - in a Raft-replicated backend fn runs on
// every replica at apply time and must be deterministic.
type TxStore interface {
	WithTx(ctx context.Context, fn func(tx Storage) error) error
}

// Storage aggregates all storage capabilities for domain entities.
type Storage interface {
	CredentialStore
	FilterFactory
	VerifierStore
	SessionStore
	RolloutStore
	AgentStore
	AgentCredentialStore
	RoleStore
	UserStore
	SpecStore
	TxStore
}
