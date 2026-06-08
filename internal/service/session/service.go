package session

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"
	"github.com/soltiHQ/control-plane/domain/model"

	"github.com/soltiHQ/control-plane/internal/service"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// Service implements session-related use-cases on top of storage contracts.
type Service struct {
	logger zerolog.Logger
	store  storage.SessionStore
}

// New creates a new sessions service.
func New(store storage.SessionStore, logger zerolog.Logger) *Service {
	if store == nil {
		panic("session.Service: store is nil")
	}
	return &Service{
		logger: logger.With().Str("service", "sessions").Logger(),
		store:  store,
	}
}

// Get returns a single session by ID.
func (s *Service) Get(ctx context.Context, id string) (*model.Session, error) {
	if id == "" {
		return nil, storage.ErrInvalidArgument
	}
	sess, err := s.store.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, storage.ErrInternal
	}
	return sess.Clone(), nil
}

// ListByUser returns all sessions for a user.
func (s *Service) ListByUser(ctx context.Context, q ListByUserQuery) (*Page, error) {
	if q.UserID == "" {
		return nil, storage.ErrInvalidArgument
	}

	items, err := s.store.ListSessionsByUser(ctx, q.UserID)
	if err != nil {
		return nil, err
	}

	limit := service.NormalizeListLimit(q.Limit, defaultListLimit)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	out := make([]*model.Session, 0, len(items))
	for _, sess := range items {
		if sess == nil {
			continue
		}
		out = append(out, sess.Clone())
	}
	return &Page{Items: out}, nil
}

// Delete deletes a single session by ID.
func (s *Service) Delete(ctx context.Context, req DeleteRequest) error {
	if req.ID == "" {
		return storage.ErrInvalidArgument
	}
	err := s.store.DeleteSession(ctx, req.ID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	s.logger.Debug().Str("session_id", req.ID).Msg("session deleted")
	return nil
}

// DeleteByUser deletes all sessions for a user.
func (s *Service) DeleteByUser(ctx context.Context, req DeleteByUserRequest) error {
	if req.UserID == "" {
		return storage.ErrInvalidArgument
	}
	if err := s.store.DeleteSessionsByUser(ctx, req.UserID); err != nil {
		return err
	}

	s.logger.Debug().Str("user_id", req.UserID).Msg("user sessions deleted")
	return nil
}

// Revoke marks a session as revoked (idempotent).
func (s *Service) Revoke(ctx context.Context, req RevokeRequest) error {
	if req.ID == "" {
		return storage.ErrInvalidArgument
	}

	at := req.At
	if at.IsZero() {
		at = time.Now()
	}

	err := s.store.RevokeSession(ctx, req.ID, at)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	s.logger.Debug().Str("session_id", req.ID).Msg("session revoked")
	return nil
}
