package session

import (
	"time"

	"github.com/soltiHQ/control-plane/domain/model"
)

const defaultListLimit = 200

// ListByUserQuery describes listing sessions for a user.
// Storage contract is non-paginated, keep Limit to prevent footguns.
type ListByUserQuery struct {
	Limit  int
	UserID string
}

// Page is a list result.
type Page struct {
	Items []*model.Session
}

// RevokeRequest describes revoking a session by ID.
type RevokeRequest struct {
	At time.Time
	ID string
}

// DeleteRequest describes deleting a single session by ID.
type DeleteRequest struct {
	ID string
}

// DeleteByUserRequest describes deleting all sessions for a user.
type DeleteByUserRequest struct {
	UserID string
}
