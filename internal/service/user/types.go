package user

import (
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/storage"
)

const defaultListLimit = 60

// ListQuery describes a paginated users listing request.
type ListQuery struct {
	// Filter is a storage-level filter.
	// Backends validate that the filter was constructed for that backend and return storage.ErrInvalidArgument otherwise.
	Filter storage.UserFilter

	Cursor string
	Limit  int
}

// Page is a paginated users listing result.
type Page struct {
	Items      []*model.User
	NextCursor string
}
