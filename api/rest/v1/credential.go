package restv1

// SetPasswordRequest describes a password change request.
type SetPasswordRequest struct {
	Password string `json:"password"`
}
