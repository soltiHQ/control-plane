package credential

import (
	"context"
	"errors"

	"github.com/rs/zerolog"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/auth"
	authcred "github.com/soltiHQ/control-plane/internal/auth/credentials"
	"github.com/soltiHQ/control-plane/internal/service"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// Service provides credential management operations.
type Service struct {
	logger zerolog.Logger
	store  storage.Storage
}

// New creates a new credential service.
func New(store storage.Storage, logger zerolog.Logger) *Service {
	if store == nil {
		panic("credential.Service: store is nil")
	}
	return &Service{
		logger: logger.With().Str("service", "credentials").Logger(),
		store:  store,
	}
}

// ListByUser returns credentials bound to a user.
func (s *Service) ListByUser(ctx context.Context, q ListByUserQuery) (*Page, error) {
	if q.UserID == "" {
		return nil, storage.ErrInvalidArgument
	}

	items, err := s.store.ListCredentialsByUser(ctx, q.UserID)
	if err != nil {
		return nil, err
	}
	limit := service.NormalizeListLimit(q.Limit, defaultListLimit)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return &Page{Items: items}, nil
}

// Get returns a single credential by ID.
func (s *Service) Get(ctx context.Context, id string) (*model.Credential, error) {
	if id == "" {
		return nil, storage.ErrInvalidArgument
	}
	return s.store.GetCredential(ctx, id)
}

// Delete removes a credential by ID and cascades verifier deletion.
func (s *Service) Delete(ctx context.Context, req DeleteRequest) error {
	if req.ID == "" {
		return storage.ErrInvalidArgument
	}

	err := s.store.WithTx(ctx, func(tx storage.Storage) error {
		if err := tx.DeleteVerifierByCredential(ctx, req.ID); err != nil {
			return err
		}
		if err := tx.DeleteCredential(ctx, req.ID); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.logger.Debug().Str("credential_id", req.ID).Msg("credential deleted")
	return nil
}

// SetPassword creates or replaces password auth material for a user.
func (s *Service) SetPassword(ctx context.Context, req SetPasswordRequest) error {
	if req.UserID == "" || req.Password == "" {
		return auth.ErrInvalidRequest
	}

	var credID string
	err := s.store.WithTx(ctx, func(tx storage.Storage) error {
		u, err := tx.GetUser(ctx, req.UserID)
		if err != nil {
			return err
		}
		if u == nil {
			return auth.ErrInvalidRequest
		}
		if u.Disabled() {
			return auth.ErrUserDisabled
		}

		credID = req.CredentialID
		if credID == "" {
			existing, err := tx.GetCredentialByUserAndAuth(ctx, req.UserID, enum.Password)
			if err != nil {
				if !errors.Is(err, storage.ErrNotFound) {
					return err
				}
				credID = "cred-" + req.UserID
			} else {
				credID = existing.ID()
			}
		}

		cred, err := model.NewCredential(credID, req.UserID, enum.Password)
		if err != nil {
			return storage.ErrInvalidArgument
		}
		if err = tx.UpsertCredential(ctx, cred); err != nil {
			return err
		}

		verifierID := "ver-" + credID
		ver, err := authcred.NewPasswordVerifier(verifierID, credID, req.Password, req.Cost)
		if err != nil {
			return err
		}
		if err = tx.DeleteVerifierByCredential(ctx, credID); err != nil {
			return err
		}
		return tx.UpsertVerifier(ctx, ver)
	})
	if err != nil {
		return err
	}

	s.logger.Debug().
		Str("user_id", req.UserID).
		Str("credential_id", credID).
		Msg("password set")
	return nil
}
