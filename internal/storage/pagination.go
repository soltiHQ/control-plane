package storage

const (
	// DefaultListLimit defines the default page size when Limit is zero or invalid.
	DefaultListLimit = 100

	// MaxListLimit defines the maximum allowed page size to prevent resource exhaustion.
	MaxListLimit = 500
)

// ListOptions specifies pagination parameters for list operations.
//
// Contract:
//   - Cursor is an opaque, implementation-defined continuation token.
//   - Limit must be clamped to (1..MaxListLimit). If zero or negative,
//     DefaultListLimit must be used.
//   - Implementations must guarantee stable ordering across pages.
//
// Ordering:
// All list operations must order results by: CreatedAt DESC, ID ASC
//
// CreatedAt is immutable, so a record updated between page fetches keeps its position.
// This ensures:
//   - Deterministic ordering.
//   - Stable cursor-based pagination.
//   - No duplicates or gaps between pages.
type ListOptions struct {
	// Cursor is an opaque continuation token returned from a previous list call.
	Cursor string

	// Limit specifies the maximum number of items to return.
	// If zero or invalid, DefaultListLimit is applied.
	Limit int
}

// ListResult contains a page of results from a list operation.
//
// Contract:
//
//   - Items must be ordered according to the storage ordering rules.
//   - NextCursor must be empty when there are no more results.
//   - NextCursor must be treated as opaque by callers.
type ListResult[T any] struct {
	// Items contain the requested page of items.
	// May be nil or empty if no items match the query.
	Items []T

	// NextCursor is an opaque token for retrieving the next page.
	// Empty string indicates this is the final page.
	NextCursor string
}

// NormalizeLimit clamps the provided limit to valid bounds.
func NormalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}
