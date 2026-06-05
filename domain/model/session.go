package model

import (
	"bytes"
	"time"

	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/enum"
)

var _ domain.Entity[*Session] = (*Session)(nil)

// Session represents an authenticated session issued by the system.
type Session struct {
	createdAt time.Time
	updatedAt time.Time
	expiresAt time.Time
	revokedAt time.Time

	id           string
	userID       string
	credentialID string

	auth        enum.Auth
	refreshHash []byte
}

// NewSession creates a new session entity.
func NewSession(id, userID, credentialID string, auth enum.Auth, refreshHash []byte, expiresAt time.Time) (*Session, error) {
	if id == "" {
		return nil, domain.ErrEmptyID
	}
	if userID == "" {
		return nil, domain.ErrEmptyUserID
	}
	if credentialID == "" {
		return nil, domain.ErrFieldEmpty
	}
	if len(refreshHash) == 0 {
		return nil, domain.ErrFieldEmpty
	}
	if expiresAt.IsZero() {
		return nil, domain.ErrFieldEmpty
	}

	var (
		now = time.Now()
		rh  = append([]byte(nil), refreshHash...)
	)
	return &Session{
		credentialID: credentialID,
		expiresAt:    expiresAt,
		userID:       userID,
		auth:         auth,
		createdAt:    now,
		updatedAt:    now,
		id:           id,
		refreshHash:  rh,
	}, nil
}

// ID returns the unique identifier of the session.
func (s *Session) ID() string { return s.id }

// UserID returns the identifier of the user this session belongs to.
func (s *Session) UserID() string { return s.userID }

// CredentialID returns the credential identifier used to create this session.
func (s *Session) CredentialID() string { return s.credentialID }

// AuthKind returns the authentication kind used for this session.
func (s *Session) AuthKind() enum.Auth { return s.auth }

// RefreshHash returns a copy of the stored refresh token hash.
func (s *Session) RefreshHash() []byte {
	return append([]byte(nil), s.refreshHash...)
}

// ExpiresAt returns the session expiration timestamp.
func (s *Session) ExpiresAt() time.Time { return s.expiresAt }

// RevokedAt returns the revocation timestamp (zero value means not revoked).
func (s *Session) RevokedAt() time.Time { return s.revokedAt }

// CreatedAt returns the timestamp when the session was created.
func (s *Session) CreatedAt() time.Time { return s.createdAt }

// UpdatedAt returns the timestamp of the last modification.
func (s *Session) UpdatedAt() time.Time { return s.updatedAt }

// SetCreatedAt restores the creation timestamp (persistence hook).
func (s *Session) SetCreatedAt(t time.Time) { s.createdAt = t }

// SetUpdatedAt restores the modification timestamp (persistence hook).
func (s *Session) SetUpdatedAt(t time.Time) { s.updatedAt = t }

// SetRevokedAt restores the revocation timestamp (persistence hook).
func (s *Session) SetRevokedAt(t time.Time) { s.revokedAt = t }

// Expired reports whether the session is expired at the given time.
func (s *Session) Expired(at time.Time) bool {
	return !s.expiresAt.IsZero() && !at.Before(s.expiresAt)
}

// Revoked reports whether the session has been revoked.
func (s *Session) Revoked() bool { return !s.revokedAt.IsZero() }

// SetRefreshHash replaces the stored refresh hash.
func (s *Session) SetRefreshHash(hash []byte) error {
	if len(hash) == 0 {
		return domain.ErrFieldEmpty
	}
	if bytes.Equal(s.refreshHash, hash) {
		return nil
	}
	s.refreshHash = append(s.refreshHash[:0], hash...)
	s.updatedAt = time.Now()
	return nil
}

// SetExpiresAt updates the session expiration time.
func (s *Session) SetExpiresAt(expiresAt time.Time) error {
	if expiresAt.IsZero() {
		return domain.ErrFieldEmpty
	}
	if s.expiresAt.Equal(expiresAt) {
		return nil
	}
	s.updatedAt = time.Now()
	s.expiresAt = expiresAt
	return nil
}

// Revoke marks the session as revoked at the provided time.
func (s *Session) Revoke(at time.Time) error {
	if at.IsZero() {
		return domain.ErrFieldEmpty
	}
	if !s.revokedAt.IsZero() {
		return nil
	}
	s.updatedAt = time.Now()
	s.revokedAt = at
	return nil
}

// Clone creates a deep copy of the session entity.
func (s *Session) Clone() *Session {
	return &Session{
		createdAt:    s.createdAt,
		updatedAt:    s.updatedAt,
		expiresAt:    s.expiresAt,
		revokedAt:    s.revokedAt,
		id:           s.id,
		userID:       s.userID,
		credentialID: s.credentialID,
		auth:         s.auth,
		refreshHash:  append([]byte(nil), s.refreshHash...),
	}
}
