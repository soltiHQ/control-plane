package restv1

import "time"

// Session is the REST representation of an authenticated session.
type Session struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at"`
	RevokedAt time.Time `json:"revoked_at,omitempty"`

	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	AuthKind     string `json:"auth_kind"`
	CredentialID string `json:"credential_id"`

	Revoked bool `json:"revoked"`
}

// ListSessionsResponse is a user's full session list (bounded; not paginated).
type ListSessionsResponse struct {
	Items []Session `json:"items"`
}
