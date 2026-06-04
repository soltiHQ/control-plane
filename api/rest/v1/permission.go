package restv1

// ListPermissionsResponse is the list of available permissions.
type ListPermissionsResponse struct {
	Items []string `json:"items"`
}
