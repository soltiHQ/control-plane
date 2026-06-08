package access

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	iauth "github.com/soltiHQ/control-plane/internal/auth"
	"github.com/soltiHQ/control-plane/internal/auth/identity"
	"github.com/soltiHQ/control-plane/internal/auth/kit"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// Service implements shared authentication use-cases.
type Service struct {
	auth *kit.Auth

	logger zerolog.Logger
	store  storage.Storage
}

// New creates a new authentication service.
func New(authSvc *kit.Auth, store storage.Storage, logger zerolog.Logger) *Service {
	if authSvc == nil {
		panic("access.Service: nil auth service")
	}
	if store == nil {
		panic("access.Service: nil storage")
	}
	return &Service{
		logger: logger.With().Str("service", "access").Logger(),
		auth:   authSvc,
		store:  store,
	}
}

// Login authenticates a user and returns issued tokens and identity.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*identity.Identity, LoginResult, error) {
	if req.Subject == "" || req.Password == "" {
		return nil, LoginResult{}, iauth.ErrInvalidRequest
	}

	s.logger.Debug().Str("subject", req.Subject).Msg("login attempt")

	now := s.auth.Clock.Now()
	if s.auth.Limiter != nil && req.RateKey != "" {
		if err := s.auth.Limiter.Check(req.RateKey, now); err != nil {
			return nil, LoginResult{}, err
		}
	}

	pair, id, err := s.auth.Session.Login(ctx, enum.Password, req.Subject, req.Password)
	if err != nil {
		if s.auth.Limiter != nil && req.RateKey != "" {
			s.auth.Limiter.RecordFailure(req.RateKey, now)
		}
		return nil, LoginResult{}, err
	}
	if s.auth.Limiter != nil && req.RateKey != "" {
		s.auth.Limiter.Reset(req.RateKey)
	}
	s.logger.Info().Str("subject", req.Subject).Str("user_id", id.UserID).Msg("login success")
	return id, LoginResult{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		SessionID:    id.SessionID,
	}, nil
}

// GetPermissions returns all available permissions in the system.
func (s *Service) GetPermissions() []enum.Permission {
	return enum.All
}

// GetRoles returns all roles from storage.
func (s *Service) GetRoles(ctx context.Context) ([]*model.Role, error) {
	res, err := s.store.ListRoles(ctx, nil, storage.ListOptions{Limit: 0})
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

// Logout revokes a session.
func (s *Service) Logout(ctx context.Context, req LogoutRequest) error {
	if req.SessionID == "" {
		return nil
	}
	if err := s.auth.Session.Revoke(ctx, req.SessionID); err != nil {
		return err
	}

	s.logger.Debug().Str("session_id", req.SessionID).Msg("logout")
	return nil
}
