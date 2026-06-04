package restv1

// Role is the REST representation of a permission role.
type Role struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListRolesResponse is the full list of roles (bounded; not paginated).
type ListRolesResponse struct {
	Items []Role `json:"items"`
}
