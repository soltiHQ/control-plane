package role

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/service"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// Service provides role management operations.
type Service struct {
	logger zerolog.Logger
	store  storage.RoleStore
}

// New creates a new role service.
func New(store storage.RoleStore, logger zerolog.Logger) *Service {
	if store == nil {
		panic("role.Service: store is nil")
	}
	return &Service{
		logger: logger.With().Str("service", "roles").Logger(),
		store:  store,
	}
}

// List returns a page of roles matching the query.
func (s *Service) List(ctx context.Context, q ListQuery) (*Page, error) {
	res, err := s.store.ListRoles(ctx, q.Filter, storage.ListOptions{
		Limit:  service.NormalizeListLimit(q.Limit, defaultListLimit),
		Cursor: q.Cursor,
	})
	if err != nil {
		return nil, err
	}

	out := make([]*model.Role, 0, len(res.Items))
	for _, r := range res.Items {
		if r == nil {
			continue
		}
		out = append(out, r.Clone())
	}
	return &Page{Items: out, NextCursor: res.NextCursor}, nil
}

// Get returns a single role by ID.
func (s *Service) Get(ctx context.Context, id string) (*model.Role, error) {
	if id == "" {
		return nil, storage.ErrInvalidArgument
	}
	r, err := s.store.GetRole(ctx, id)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, storage.ErrInternal
	}
	return r.Clone(), nil
}

// Upsert creates or replaces a role.
func (s *Service) Upsert(ctx context.Context, r *model.Role) error {
	if r == nil {
		return storage.ErrInvalidArgument
	}
	if err := s.store.UpsertRole(ctx, r); err != nil {
		return err
	}

	s.logger.Debug().Str("role_id", r.ID()).Msg("role upserted")
	return nil
}

// Delete removes a role by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	if id == "" {
		return storage.ErrInvalidArgument
	}
	if err := s.store.DeleteRole(ctx, id); err != nil {
		return err
	}

	s.logger.Debug().Str("role_id", id).Msg("role deleted")
	return nil
}
