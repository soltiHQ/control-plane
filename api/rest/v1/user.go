package restv1

// User is the REST representation of a platform user.
type User struct {
	Permissions []string `json:"permissions,omitempty"`
	RoleNames   []string `json:"role_names,omitempty"`
	RoleIDs     []string `json:"role_ids,omitempty"`

	ID      string `json:"id"`
	Subject string `json:"subject"`
	Name    string `json:"name,omitempty"`
	Email   string `json:"email,omitempty"`

	Disabled bool `json:"disabled"`
}

// ListUsersResponse is the paginated list of users.
type ListUsersResponse struct {
	Items      []User `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}
