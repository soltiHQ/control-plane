package errkind

import (
	"context"
	"errors"

	"github.com/soltiHQ/control-plane/internal/auth"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// Kind is a transport-agnostic error category.
type Kind int

const (
	Internal Kind = iota
	InvalidArgument
	Unauthenticated
	PermissionDenied
	NotFound
	AlreadyExists
	Conflict
	FailedPrecondition
	Canceled
	DeadlineExceeded
	Unavailable
)

// Classify maps a (non-nil) error to its Kind.
func Classify(err error) Kind {
	switch {
	case err == nil:
		return Internal

	case errors.Is(err, context.Canceled):
		return Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return DeadlineExceeded

	case errors.Is(err, auth.ErrInvalidCredentials),
		errors.Is(err, auth.ErrPasswordMismatch),
		errors.Is(err, auth.ErrInvalidToken),
		errors.Is(err, auth.ErrExpiredToken),
		errors.Is(err, auth.ErrInvalidRefresh),
		errors.Is(err, auth.ErrRevoked):
		return Unauthenticated

	case errors.Is(err, auth.ErrUnauthorized):
		return PermissionDenied

	case errors.Is(err, auth.ErrInvalidRequest),
		errors.Is(err, auth.ErrInvalidArgument),
		errors.Is(err, auth.ErrWrongAuthKind):
		return InvalidArgument

	case errors.Is(err, auth.ErrUserDisabled):
		return FailedPrecondition

	case errors.Is(err, storage.ErrNotFound):
		return NotFound
	case errors.Is(err, storage.ErrAlreadyExists):
		return AlreadyExists
	case errors.Is(err, storage.ErrConflict):
		return Conflict
	case errors.Is(err, storage.ErrInvalidArgument):
		return InvalidArgument
	case errors.Is(err, storage.ErrUnavailable):
		return Unavailable

	default:
		return Internal
	}
}
