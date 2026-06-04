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

// ListUsersResponse is a cursor-paginated list of users (next_cursor).
type ListUsersResponse struct {
	Items      []User `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// SetPasswordRequest is the request body for POST /users/{id}/password.
type SetPasswordRequest struct {
	Password string `json:"password"`
}
