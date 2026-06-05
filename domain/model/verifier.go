package model

import (
	"time"

	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/enum"
)

var _ domain.Entity[*Verifier] = (*Verifier)(nil)

// Verifier stores authentication verification material for a credential.
//
// Verifier is security-sensitive data used to verify an authentication attempt.
// It is separated from Credential to keep different lifecycles isolated:
//   - Credential: binds user <-> auth kind (rarely changes)
//   - Verifier: stores verifier payload (changes on password reset / key rotation)
//
// Notes:
//   - Never store raw secrets (passwords, API keys). Store hashes/params instead.
//   - Data layout depends on Auth kind (password/api_key/etc).
type Verifier struct {
	createdAt time.Time
	updatedAt time.Time

	id           string
	credentialID string

	auth enum.Auth

	// Contains auth-kind specific verifier payload (e.g., password hash/params).
	data map[string]string
}

// NewVerifier creates a new verifier entity.
func NewVerifier(id, credentialID string, auth enum.Auth) (*Verifier, error) {
	if id == "" {
		return nil, domain.ErrEmptyID
	}
	if credentialID == "" {
		return nil, domain.ErrFieldEmpty
	}
	if auth == "" {
		return nil, domain.ErrFieldEmpty
	}

	now := time.Now()
	return &Verifier{
		createdAt:    now,
		updatedAt:    now,
		id:           id,
		credentialID: credentialID,
		auth:         auth,
		data:         make(map[string]string),
	}, nil
}

// ID returns the unique identifier of the verifier.
func (v *Verifier) ID() string { return v.id }

// CredentialID returns the credential identifier this verifier belongs to.
func (v *Verifier) CredentialID() string { return v.credentialID }

// AuthKind returns the authentication kind of this verifier.
func (v *Verifier) AuthKind() enum.Auth { return v.auth }

// CreatedAt returns the timestamp when the verifier was created.
func (v *Verifier) CreatedAt() time.Time { return v.createdAt }

// UpdatedAt returns the timestamp of the last modification.
func (v *Verifier) UpdatedAt() time.Time { return v.updatedAt }

// SetCreatedAt / SetUpdatedAt — used by persistence adapters.
func (v *Verifier) SetCreatedAt(t time.Time) { v.createdAt = t }
func (v *Verifier) SetUpdatedAt(t time.Time) { v.updatedAt = t }

// DataGet returns a verifier data value by key.
func (v *Verifier) DataGet(key string) (string, bool) {
	val, ok := v.data[key]
	return val, ok
}

// DataAll returns a copy of verifier data.
func (v *Verifier) DataAll() map[string]string {
	out := make(map[string]string, len(v.data))
	for k, x := range v.data {
		out[k] = x
	}
	return out
}

// DataSet sets a verifier data value and updates UpdatedAt.
func (v *Verifier) DataSet(key, value string) error {
	if key == "" || value == "" {
		return domain.ErrFieldEmpty
	}
	if v.data == nil {
		v.data = make(map[string]string)
	}
	if existing, ok := v.data[key]; ok && existing == value {
		return nil
	}
	v.data[key] = value
	v.updatedAt = time.Now()
	return nil
}

// DataDelete removes a verifier data key and bumps UpdatedAt if it existed.
//
// The operation is idempotent: deleting a missing key is a no-op.
func (v *Verifier) DataDelete(key string) error {
	if key == "" {
		return domain.ErrFieldEmpty
	}
	if v.data == nil {
		return nil
	}
	if _, ok := v.data[key]; !ok {
		return nil
	}
	delete(v.data, key)
	v.updatedAt = time.Now()
	return nil
}

// Clone creates a deep copy of the verifier entity.
func (v *Verifier) Clone() *Verifier {
	out := make(map[string]string, len(v.data))
	for k, x := range v.data {
		out[k] = x
	}

	return &Verifier{
		createdAt:    v.createdAt,
		updatedAt:    v.updatedAt,
		id:           v.id,
		credentialID: v.credentialID,
		auth:         v.auth,
		data:         out,
	}
}
