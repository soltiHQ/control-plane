package agent

import (
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/storage"
)

const defaultListLimit = 30

// ListQuery describes a paginated agents listing request.
type ListQuery struct {
	// Filter is a storage-level filter.
	// Backends validate that the filter was constructed for that backend and return storage.ErrInvalidArgument otherwise.
	Filter storage.AgentFilter

	Cursor string
	Limit  int
}

// Page is a paginated agents listing result.
type Page struct {
	Items      []*model.Agent
	NextCursor string
}

// PatchLabels updates control-plane owned labels for an agent.
//
// Semantics:
//   - ID is required.
//   - Labels replace the entire label set (no merge).
type PatchLabels struct {
	Labels map[string]string
	ID     string
}
