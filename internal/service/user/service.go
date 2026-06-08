package user

import (
	"context"
	"errors"
	"strings"

	"github.com/rs/zerolog"
	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/service"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// Service implements user-related use-cases on top of storage contracts.
type Service struct {
	logger zerolog.Logger
	store  storage.Storage
}

// New creates a new users service.
func New(store storage.Storage, logger zerolog.Logger) *Service {
	if store == nil {
		panic("user.Service: store is nil")
	}
	return &Service{
		logger: logger.With().Str("service", "users").Logger(),
		store:  store,
	}
}

// List returns a page of users matching the query.
func (s *Service) List(ctx context.Context, q ListQuery) (*Page, error) {
	res, err := s.store.ListUsers(ctx, q.Filter, storage.ListOptions{
		Limit:  service.NormalizeListLimit(q.Limit, defaultListLimit),
		Cursor: q.Cursor,
	})
	if err != nil {
		return nil, err
	}
	return &Page{Items: res.Items, NextCursor: res.NextCursor}, nil
}

// Get returns a single user by ID.
func (s *Service) Get(ctx context.Context, id string) (*model.User, error) {
	if id == "" {
		return nil, storage.ErrInvalidArgument
	}
	return s.store.GetUser(ctx, id)
}

// GetBySubject returns a single user by authentication subject.
func (s *Service) GetBySubject(ctx context.Context, subject string) (*model.User, error) {
	if subject == "" {
		return nil, storage.ErrInvalidArgument
	}
	return s.store.GetUserBySubject(ctx, subject)
}

// Delete a user by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	if id == "" {
		return storage.ErrInvalidArgument
	}

	var credCount int
	err := s.store.WithTx(ctx, func(tx storage.Storage) error {
		if err := tx.DeleteSessionsByUser(ctx, id); err != nil {
			return err
		}
		creds, err := tx.ListCredentialsByUser(ctx, id)
		if err != nil {
			return err
		}
		credCount = len(creds)
		for _, c := range creds {
			if c == nil {
				continue
			}
			if err = tx.DeleteVerifierByCredential(ctx, c.ID()); err != nil {
				return err
			}
			if err = tx.DeleteCredential(ctx, c.ID()); err != nil && !errors.Is(err, storage.ErrNotFound) {
				return err
			}
		}
		if err = tx.DeleteUser(ctx, id); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		return nil
	})
	if err != nil {
		s.logger.Warn().Err(err).Str("user_id", id).Msg("delete failed")
		return err
	}
	s.logger.Debug().Str("user_id", id).Int("credentials", credCount).Msg("user deleted")
	return nil
}

// Upsert creates or replaces a user.
func (s *Service) Upsert(ctx context.Context, u *model.User) error {
	if u == nil {
		return storage.ErrInvalidArgument
	}

	subject := strings.TrimSpace(strings.ToLower(u.Subject()))
	if subject == "" {
		return domain.ErrInvalidSubject
	}
	u.SetSubject(subject)
	u.SetName(strings.TrimSpace(u.Name()))

	email := strings.TrimSpace(strings.ToLower(u.Email()))
	if email != "" && !isValidEmail(email) {
		return domain.ErrInvalidEmail
	}
	u.SetEmail(email)

	err := s.store.WithTx(ctx, func(tx storage.Storage) error {
		switch existing, err := tx.GetUserBySubject(ctx, subject); {
		case err == nil:
			if existing.ID() != u.ID() {
				return storage.ErrAlreadyExists
			}
		case !errors.Is(err, storage.ErrNotFound):
			return err
		}
		if ids := u.RoleIDsAll(); len(ids) > 0 {
			if _, err := tx.GetRoles(ctx, ids); err != nil {
				return err
			}
		}
		return tx.UpsertUser(ctx, u)
	})
	if err != nil {
		return err
	}

	s.logger.Debug().
		Str("user_id", u.ID()).
		Str("subject", subject).
		Msg("user upserted")
	return nil
}

// isValidEmail performs a basic email format check:
func isValidEmail(email string) bool {
	at := strings.IndexByte(email, '@')
	if at < 1 || at == len(email)-1 {
		return false
	}
	d := email[at+1:]
	return strings.ContainsRune(d, '.') && !strings.HasSuffix(d, ".")
}
