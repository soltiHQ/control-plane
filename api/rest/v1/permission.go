package restv1

// ListPermissionsResponse is the full list of available permissions (bounded; not paginated).
type ListPermissionsResponse struct {
	Items []string `json:"items"`
}
