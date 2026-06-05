// Package bootstrap seeds initial data required for the control-plane.
package bootstrap

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/auth/credentials"
	"github.com/soltiHQ/control-plane/internal/service/credential"
	"github.com/soltiHQ/control-plane/internal/service/role"
	"github.com/soltiHQ/control-plane/internal/service/user"
)

const (
	adminUserID  = "user-admin"
	adminSubject = "admin"
)

// Run service bootstrap operations.
func Run(ctx context.Context, logger zerolog.Logger, roleSVC *role.Service, userSVC *user.Service, credSVC *credential.Service) error {
	logger.Debug().Msg("bootstrap: starting")
	if err := seedRoles(ctx, logger, roleSVC); err != nil {
		return err
	}
	return seedAdmin(ctx, logger, userSVC, credSVC)
}

func seedRoles(ctx context.Context, logger zerolog.Logger, roleSVC *role.Service) error {
	for _, br := range enum.BuiltinRoles {
		r, err := model.NewRole(br.ID, br.Name)
		if err != nil {
			return err
		}

		for _, p := range br.Permissions {
			if err = r.PermissionAdd(p); err != nil {
				return err
			}
		}
		if err = roleSVC.Upsert(ctx, r); err != nil {
			return err
		}
		logger.Debug().Str("role_id", br.ID).Str("name", br.Name).Msg("bootstrap: role seeded")
	}
	return nil
}

func seedAdmin(ctx context.Context, logger zerolog.Logger, userSVC *user.Service, credSVC *credential.Service) error {
	u, err := model.NewUser(adminUserID, adminSubject)
	if err != nil {
		return err
	}
	u.SetEmail("admin@solit.local")
	u.SetName("admin")

	if err = u.RoleAdd(enum.RoleAdminID); err != nil {
		return err
	}
	if err = userSVC.Upsert(ctx, u); err != nil {
		return err
	}
	logger.Debug().Str("user_id", adminUserID).Msg("bootstrap: admin user seeded")

	password, err := credentials.GeneratePassword(0)
	if err != nil {
		return err
	}

	if err = credSVC.SetPassword(ctx, credential.SetPasswordRequest{
		UserID:   adminUserID,
		Password: password,
	}); err != nil {
		return err
	}

	logger.Warn().
		Str("login", "admin").
		Str("password", password).
		Msg("bootstrap: admin user created: change the password after first login")
	return nil
}
