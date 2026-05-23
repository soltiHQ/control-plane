package raft

import (
	"context"
	"time"

	genv1 "github.com/soltiHQ/control-plane/api/gen/v1"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/domain/wire"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// txView is a recording storage.Storage used inside Store.WithTx. It
// captures every mutation as a proto Op for later batch submission to
// Raft, and overlays pending writes on top of the underlying store so fn
// sees its own mutations on subsequent reads.
//
// The overlay is intentionally simple: only the most recent Upsert per
// (kind, id) is remembered, and Deletes poison reads. Fancier isolation
// (range queries reflecting overlay) is NOT supported — List* operations
// bypass the overlay. That's fine in practice: nothing inside the
// services' WithTx closures reads a listing after writing to it.
type txView struct {
	inner storage.Storage
	ops   []*genv1.Op

	// Per-entity overlays. key = ID; value = pointer or nil (nil means
	// "deleted within this tx").
	agents      map[string]*model.Agent
	users       map[string]*model.User
	roles       map[string]*model.Role
	credentials map[string]*model.Credential
	verifiers   map[string]*model.Verifier
	sessions    map[string]*model.Session
	specs       map[string]*model.Spec
	rollouts    map[string]*model.Rollout
}

func newTxView(inner storage.Storage) *txView {
	return &txView{
		inner:       inner,
		agents:      map[string]*model.Agent{},
		users:       map[string]*model.User{},
		roles:       map[string]*model.Role{},
		credentials: map[string]*model.Credential{},
		verifiers:   map[string]*model.Verifier{},
		sessions:    map[string]*model.Session{},
		specs:       map[string]*model.Spec{},
		rollouts:    map[string]*model.Rollout{},
	}
}

// overlayGet returns (value, found, tombstoned). If found is false, caller
// falls through to inner.
func overlayGet[T any](m map[string]T, id string) (T, bool, bool) {
	v, ok := m[id]
	if !ok {
		var zero T
		return zero, false, false
	}
	// Type-erase nil check: for pointer types, v==zero means tombstoned.
	var zero T
	isNil := any(v) == any(zero)
	return v, true, isNil
}

// === Agents ===

func (v *txView) UpsertAgent(ctx context.Context, a *model.Agent) error {
	if a == nil {
		return storage.ErrInvalidArgument
	}
	v.agents[a.ID()] = a
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_AgentUpsert{AgentUpsert: wire.AgentToProto(a)}})
	return nil
}

func (v *txView) GetAgent(ctx context.Context, id string) (*model.Agent, error) {
	if val, found, tomb := overlayGet(v.agents, id); found {
		if tomb {
			return nil, storage.ErrNotFound
		}
		return val, nil
	}
	return v.inner.GetAgent(ctx, id)
}

func (v *txView) ListAgents(ctx context.Context, f storage.AgentFilter, o storage.ListOptions) (*storage.AgentListResult, error) {
	return v.inner.ListAgents(ctx, f, o)
}

func (v *txView) DeleteAgent(ctx context.Context, id string) error {
	v.agents[id] = nil
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_AgentDelete{AgentDelete: id}})
	return nil
}

// === Users ===

func (v *txView) UpsertUser(ctx context.Context, u *model.User) error {
	if u == nil {
		return storage.ErrInvalidArgument
	}
	v.users[u.ID()] = u
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_UserUpsert{UserUpsert: wire.UserToProto(u)}})
	return nil
}

func (v *txView) GetUser(ctx context.Context, id string) (*model.User, error) {
	if val, found, tomb := overlayGet(v.users, id); found {
		if tomb {
			return nil, storage.ErrNotFound
		}
		return val, nil
	}
	return v.inner.GetUser(ctx, id)
}

func (v *txView) GetUserBySubject(ctx context.Context, sub string) (*model.User, error) {
	return v.inner.GetUserBySubject(ctx, sub)
}

func (v *txView) ListUsers(ctx context.Context, f storage.UserFilter, o storage.ListOptions) (*storage.UserListResult, error) {
	return v.inner.ListUsers(ctx, f, o)
}

func (v *txView) DeleteUser(ctx context.Context, id string) error {
	v.users[id] = nil
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_UserDelete{UserDelete: id}})
	return nil
}

// === Roles ===

func (v *txView) UpsertRole(ctx context.Context, r *model.Role) error {
	if r == nil {
		return storage.ErrInvalidArgument
	}
	v.roles[r.ID()] = r
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_RoleUpsert{RoleUpsert: wire.RoleToProto(r)}})
	return nil
}

func (v *txView) GetRole(ctx context.Context, id string) (*model.Role, error) {
	if val, found, tomb := overlayGet(v.roles, id); found {
		if tomb {
			return nil, storage.ErrNotFound
		}
		return val, nil
	}
	return v.inner.GetRole(ctx, id)
}

func (v *txView) GetRoles(ctx context.Context, ids []string) ([]*model.Role, error) {
	return v.inner.GetRoles(ctx, ids)
}

func (v *txView) GetRoleByName(ctx context.Context, name string) (*model.Role, error) {
	return v.inner.GetRoleByName(ctx, name)
}

func (v *txView) ListRoles(ctx context.Context, f storage.RoleFilter, o storage.ListOptions) (*storage.RoleListResult, error) {
	return v.inner.ListRoles(ctx, f, o)
}

func (v *txView) DeleteRole(ctx context.Context, id string) error {
	v.roles[id] = nil
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_RoleDelete{RoleDelete: id}})
	return nil
}

// === Credentials ===

func (v *txView) UpsertCredential(ctx context.Context, c *model.Credential) error {
	if c == nil {
		return storage.ErrInvalidArgument
	}
	v.credentials[c.ID()] = c
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_CredentialUpsert{CredentialUpsert: wire.CredentialToProto(c)}})
	return nil
}

func (v *txView) GetCredential(ctx context.Context, id string) (*model.Credential, error) {
	if val, found, tomb := overlayGet(v.credentials, id); found {
		if tomb {
			return nil, storage.ErrNotFound
		}
		return val, nil
	}
	return v.inner.GetCredential(ctx, id)
}

func (v *txView) GetCredentialByUserAndAuth(ctx context.Context, userID string, auth enum.Auth) (*model.Credential, error) {
	return v.inner.GetCredentialByUserAndAuth(ctx, userID, auth)
}

func (v *txView) ListCredentialsByUser(ctx context.Context, userID string) ([]*model.Credential, error) {
	return v.inner.ListCredentialsByUser(ctx, userID)
}

func (v *txView) DeleteCredential(ctx context.Context, id string) error {
	v.credentials[id] = nil
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_CredentialDelete{CredentialDelete: id}})
	return nil
}

// === Verifiers ===

func (v *txView) UpsertVerifier(ctx context.Context, ver *model.Verifier) error {
	if ver == nil {
		return storage.ErrInvalidArgument
	}
	v.verifiers[ver.ID()] = ver
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_VerifierUpsert{VerifierUpsert: wire.VerifierToProto(ver)}})
	return nil
}

func (v *txView) GetVerifier(ctx context.Context, id string) (*model.Verifier, error) {
	if val, found, tomb := overlayGet(v.verifiers, id); found {
		if tomb {
			return nil, storage.ErrNotFound
		}
		return val, nil
	}
	return v.inner.GetVerifier(ctx, id)
}

func (v *txView) GetVerifierByCredential(ctx context.Context, credID string) (*model.Verifier, error) {
	return v.inner.GetVerifierByCredential(ctx, credID)
}

func (v *txView) DeleteVerifier(ctx context.Context, id string) error {
	v.verifiers[id] = nil
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_VerifierDelete{VerifierDelete: id}})
	return nil
}

func (v *txView) DeleteVerifierByCredential(ctx context.Context, credID string) error {
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_VerifierDeleteByCred{VerifierDeleteByCred: credID}})
	return nil
}

// === Sessions ===

func (v *txView) CreateSession(ctx context.Context, s *model.Session) error {
	if s == nil {
		return storage.ErrInvalidArgument
	}
	v.sessions[s.ID()] = s
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_SessionCreate{SessionCreate: wire.SessionToProto(s)}})
	return nil
}

func (v *txView) GetSession(ctx context.Context, id string) (*model.Session, error) {
	if val, found, tomb := overlayGet(v.sessions, id); found {
		if tomb {
			return nil, storage.ErrNotFound
		}
		return val, nil
	}
	return v.inner.GetSession(ctx, id)
}

func (v *txView) ListSessionsByUser(ctx context.Context, userID string) ([]*model.Session, error) {
	return v.inner.ListSessionsByUser(ctx, userID)
}

func (v *txView) RotateRefresh(ctx context.Context, sessionID string, newHash []byte, newExpiresAt time.Time) error {
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_SessionRotateRefresh{
		SessionRotateRefresh: &genv1.SessionRotateRefreshMsg{
			Id:          sessionID,
			RefreshHash: append([]byte(nil), newHash...),
			ExpiresAtNs: newExpiresAt.UnixNano(),
		},
	}})
	return nil
}

func (v *txView) RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error {
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_SessionRevoke{
		SessionRevoke: &genv1.SessionRevokeMsg{Id: sessionID, RevokedAtNs: revokedAt.UnixNano()},
	}})
	return nil
}

func (v *txView) DeleteSession(ctx context.Context, id string) error {
	v.sessions[id] = nil
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_SessionDelete{SessionDelete: id}})
	return nil
}

func (v *txView) DeleteSessionsByUser(ctx context.Context, userID string) error {
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_SessionDeleteByUser{SessionDeleteByUser: userID}})
	return nil
}

// === Specs ===

func (v *txView) UpsertSpec(ctx context.Context, ts *model.Spec) error {
	if ts == nil {
		return storage.ErrInvalidArgument
	}
	v.specs[ts.ID()] = ts
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_SpecUpsert{SpecUpsert: wire.SpecToProto(ts)}})
	return nil
}

func (v *txView) GetSpec(ctx context.Context, id string) (*model.Spec, error) {
	if val, found, tomb := overlayGet(v.specs, id); found {
		if tomb {
			return nil, storage.ErrNotFound
		}
		return val, nil
	}
	return v.inner.GetSpec(ctx, id)
}

func (v *txView) ListSpecs(ctx context.Context, f storage.SpecFilter, o storage.ListOptions) (*storage.SpecListResult, error) {
	return v.inner.ListSpecs(ctx, f, o)
}

func (v *txView) DeleteSpec(ctx context.Context, id string) error {
	v.specs[id] = nil
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_SpecDelete{SpecDelete: id}})
	return nil
}

// === Rollouts ===

func (v *txView) UpsertRollout(ctx context.Context, r *model.Rollout) error {
	if r == nil {
		return storage.ErrInvalidArgument
	}
	v.rollouts[r.ID()] = r
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_RolloutUpsert{RolloutUpsert: wire.RolloutToProto(r)}})
	return nil
}

func (v *txView) GetRollout(ctx context.Context, id string) (*model.Rollout, error) {
	if val, found, tomb := overlayGet(v.rollouts, id); found {
		if tomb {
			return nil, storage.ErrNotFound
		}
		return val, nil
	}
	return v.inner.GetRollout(ctx, id)
}

func (v *txView) ListRollouts(ctx context.Context, f storage.RolloutFilter, o storage.ListOptions) (*storage.RolloutListResult, error) {
	return v.inner.ListRollouts(ctx, f, o)
}

func (v *txView) DeleteRollout(ctx context.Context, id string) error {
	v.rollouts[id] = nil
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_RolloutDelete{RolloutDelete: id}})
	return nil
}

func (v *txView) DeleteRolloutsBySpec(ctx context.Context, specID string) error {
	v.ops = append(v.ops, &genv1.Op{Op: &genv1.Op_RolloutDeleteBySpec{RolloutDeleteBySpec: specID}})
	return nil
}

// === FilterFactory + nested tx (no-op) ===

func (v *txView) BuildRolloutFilter(c storage.RolloutQueryCriteria) storage.RolloutFilter {
	return v.inner.BuildRolloutFilter(c)
}

func (v *txView) BuildSpecFilter(c storage.SpecQueryCriteria) storage.SpecFilter {
	return v.inner.BuildSpecFilter(c)
}

// WithTx on a txView simply invokes fn again with the same view — nested
// WithTx is a composition helper, not a true sub-transaction.
func (v *txView) WithTx(ctx context.Context, fn func(tx storage.Storage) error) error {
	if fn == nil {
		return storage.ErrInvalidArgument
	}
	return fn(v)
}
