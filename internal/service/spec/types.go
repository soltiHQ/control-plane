package spec

import (
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/storage"
)

const defaultListLimit = 30

// ListQuery describes a paginated task spec listing request.
type ListQuery struct {
	Criteria storage.SpecQueryCriteria
	Cursor   string
	Limit    int
}

// Page is a paginated task spec listing the result.
type Page struct {
	Items      []*model.Spec
	NextCursor string
}
