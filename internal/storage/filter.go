package storage

import "github.com/soltiHQ/control-plane/domain/enum"

// AgentFilter defines a backend-specific query object for agents.
//
// A filter must be constructed by the same storage backend that consumes it.
// Passing a filter created for a different backend must return ErrInvalidArgument.
//
// Note:
// This is enforced by backend implementations via type assertion to their concrete filter type (e.g. inmemory.AgentFilter).
type AgentFilter interface{}

// UserFilter defines a backend-specific query object for users.
//
// A filter must be constructed by the same storage backend that consumes it.
// Passing a filter created for a different backend must return ErrInvalidArgument.
//
// Note:
// This is enforced by backend implementations via type assertion to their concrete filter type (e.g. inmemory.UserFilter).
type UserFilter interface{}

// RoleFilter defines a backend-specific query object for roles.
//
// A filter must be constructed by the same storage backend that consumes it.
// Passing a filter created for a different backend must return ErrInvalidArgument.
//
// Note:
// This is enforced by backend implementations via type assertion to their concrete filter type (e.g. inmemory.RoleFilter).
type RoleFilter interface{}

// SpecFilter defines a backend-specific query object for specs.
type SpecFilter interface{}

// RolloutFilter defines a backend-specific query object for rollouts.
type RolloutFilter interface{}

// RolloutQueryCriteria defines a backend-agnostic query description for rollouts.
//
// Each field is optional and zero values are ignored during filter construction.
// Passing a fully zeroed struct therefore matches every rollout in storage.
//
// Note:
// This struct is consumed by FilterFactory.BuildRolloutFilter, which translates it into the backend-specific RolloutFilter.
type RolloutQueryCriteria struct {
	SpecID   string
	AgentID  string
	Statuses []enum.SyncStatus
}

// SpecQueryCriteria defines a backend-agnostic query description for specs.
//
// Each field is optional and zero values are ignored during filter construction.
// Passing a fully zeroed struct therefore matches every spec in storage.
//
// Note:
// This struct is consumed by FilterFactory.BuildSpecFilter, which translates it into the backend-specific SpecFilter.
type SpecQueryCriteria struct {
	Query string
}
