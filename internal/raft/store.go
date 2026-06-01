package raft

import (
	"context"
	"fmt"
	"sync"
	"time"

	hraft "github.com/hashicorp/raft"

	raftv1 "github.com/soltiHQ/control-plane/api/gen/solti/raft/v1"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/domain/wire"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// Compile-time check.
var _ storage.Storage = (*Store)(nil)

// applyTimeout bounds the wait for a Raft log entry to be committed.
const applyTimeout = 5 * time.Second

// Store wraps an inner storage.Storage. Reads go local (eventual
// consistency). Writes are encoded as protobuf Commands and submitted to
// raft.Apply; the FSM applies them to the inner store on every replica
// in log order.
//
// Write flow:
//
//   - Every single mutation method builds a 1-op Command and submits it.
//   - WithTx collects ops via a recording txView, then submits them as one
//     multi-op Command for atomic replication. fn's reads are served from
//     the inner store plus an in-memory overlay so within-fn read-your-writes
//     works as expected.
//
// The inner store must be the plain inmemory one and MUST be the same store
// the FSM applies against. Otherwise leader and followers diverge.
type Store struct {
	inner storage.Storage
	raft  *hraft.Raft

	txMu sync.Mutex // single in-flight WithTx at a time
}

// NewStore builds a Raft-backed Store.
func NewStore(inner storage.Storage, r *hraft.Raft) *Store {
	if inner == nil {
		panic("raft: nil inner store")
	}
	if r == nil {
		panic("raft: nil raft")
	}
	return &Store{inner: inner, raft: r}
}

// apply submits a Command to Raft, waiting up to applyTimeout for commit.
func (s *Store) apply(ops []*raftv1.Op) error {
	data, err := encodeCommand(ops)
	if err != nil {
		return err
	}
	f := s.raft.Apply(data, applyTimeout)
	if err := f.Error(); err != nil {
		return fmt.Errorf("raft: apply: %w", err)
	}
	if resp := f.Response(); resp != nil {
		if e, ok := resp.(error); ok {
			return e
		}
	}
	return nil
}

func (s *Store) applyOp(op *raftv1.Op) error { return s.apply([]*raftv1.Op{op}) }

// === Agents ===

func (s *Store) UpsertAgent(ctx context.Context, a *model.Agent) error {
	if a == nil {
		return storage.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_AgentUpsert{AgentUpsert: wire.AgentToProto(a)}})
}

func (s *Store) GetAgent(ctx context.Context, id string) (*model.Agent, error) {
	return s.inner.GetAgent(ctx, id)
}

func (s *Store) ListAgents(ctx context.Context, f storage.AgentFilter, o storage.ListOptions) (*storage.AgentListResult, error) {
	return s.inner.ListAgents(ctx, f, o)
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_AgentDelete{AgentDelete: id}})
}

// === Users ===

func (s *Store) UpsertUser(ctx context.Context, u *model.User) error {
	if u == nil {
		return storage.ErrInvalidArgument
	}
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_UserUpsert{UserUpsert: wire.UserToProto(u)}})
}

func (s *Store) GetUser(ctx context.Context, id string) (*model.User, error) {
	return s.inner.GetUser(ctx, id)
}

func (s *Store) GetUserBySubject(ctx context.Context, sub string) (*model.User, error) {
	return s.inner.GetUserBySubject(ctx, sub)
}

func (s *Store) ListUsers(ctx context.Context, f storage.UserFilter, o storage.ListOptions) (*storage.UserListResult, error) {
	return s.inner.ListUsers(ctx, f, o)
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_UserDelete{UserDelete: id}})
}

// === Roles ===

func (s *Store) UpsertRole(ctx context.Context, r *model.Role) error {
	if r == nil {
		return storage.ErrInvalidArgument
	}
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_RoleUpsert{RoleUpsert: wire.RoleToProto(r)}})
}

func (s *Store) GetRole(ctx context.Context, id string) (*model.Role, error) {
	return s.inner.GetRole(ctx, id)
}

func (s *Store) GetRoles(ctx context.Context, ids []string) ([]*model.Role, error) {
	return s.inner.GetRoles(ctx, ids)
}

func (s *Store) GetRoleByName(ctx context.Context, name string) (*model.Role, error) {
	return s.inner.GetRoleByName(ctx, name)
}

func (s *Store) ListRoles(ctx context.Context, f storage.RoleFilter, o storage.ListOptions) (*storage.RoleListResult, error) {
	return s.inner.ListRoles(ctx, f, o)
}

func (s *Store) DeleteRole(ctx context.Context, id string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_RoleDelete{RoleDelete: id}})
}

// === Credentials ===

func (s *Store) UpsertCredential(ctx context.Context, c *model.Credential) error {
	if c == nil {
		return storage.ErrInvalidArgument
	}
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_CredentialUpsert{CredentialUpsert: wire.CredentialToProto(c)}})
}

func (s *Store) GetCredential(ctx context.Context, id string) (*model.Credential, error) {
	return s.inner.GetCredential(ctx, id)
}

func (s *Store) GetCredentialByUserAndAuth(ctx context.Context, userID string, auth enum.Auth) (*model.Credential, error) {
	return s.inner.GetCredentialByUserAndAuth(ctx, userID, auth)
}

func (s *Store) ListCredentialsByUser(ctx context.Context, userID string) ([]*model.Credential, error) {
	return s.inner.ListCredentialsByUser(ctx, userID)
}

func (s *Store) DeleteCredential(ctx context.Context, id string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_CredentialDelete{CredentialDelete: id}})
}

// === Verifiers ===

func (s *Store) UpsertVerifier(ctx context.Context, v *model.Verifier) error {
	if v == nil {
		return storage.ErrInvalidArgument
	}
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_VerifierUpsert{VerifierUpsert: wire.VerifierToProto(v)}})
}

func (s *Store) GetVerifier(ctx context.Context, id string) (*model.Verifier, error) {
	return s.inner.GetVerifier(ctx, id)
}

func (s *Store) GetVerifierByCredential(ctx context.Context, credID string) (*model.Verifier, error) {
	return s.inner.GetVerifierByCredential(ctx, credID)
}

func (s *Store) DeleteVerifier(ctx context.Context, id string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_VerifierDelete{VerifierDelete: id}})
}

func (s *Store) DeleteVerifierByCredential(ctx context.Context, credID string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_VerifierDeleteByCred{VerifierDeleteByCred: credID}})
}

// === Sessions ===

func (s *Store) CreateSession(ctx context.Context, ss *model.Session) error {
	if ss == nil {
		return storage.ErrInvalidArgument
	}
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_SessionCreate{SessionCreate: wire.SessionToProto(ss)}})
}

func (s *Store) GetSession(ctx context.Context, id string) (*model.Session, error) {
	return s.inner.GetSession(ctx, id)
}

func (s *Store) ListSessionsByUser(ctx context.Context, userID string) ([]*model.Session, error) {
	return s.inner.ListSessionsByUser(ctx, userID)
}

func (s *Store) RotateRefresh(ctx context.Context, sessionID string, newHash []byte, newExpiresAt time.Time) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_SessionRotateRefresh{
		SessionRotateRefresh: &raftv1.SessionRotateRefreshMsg{
			Id:          sessionID,
			RefreshHash: append([]byte(nil), newHash...),
			ExpiresAtNs: newExpiresAt.UnixNano(),
		},
	}})
}

func (s *Store) RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_SessionRevoke{
		SessionRevoke: &raftv1.SessionRevokeMsg{Id: sessionID, RevokedAtNs: revokedAt.UnixNano()},
	}})
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_SessionDelete{SessionDelete: id}})
}

func (s *Store) DeleteSessionsByUser(ctx context.Context, userID string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_SessionDeleteByUser{SessionDeleteByUser: userID}})
}

// === Specs ===

func (s *Store) UpsertSpec(ctx context.Context, ts *model.Spec) error {
	if ts == nil {
		return storage.ErrInvalidArgument
	}
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_SpecUpsert{SpecUpsert: wire.SpecToProto(ts)}})
}

func (s *Store) GetSpec(ctx context.Context, id string) (*model.Spec, error) {
	return s.inner.GetSpec(ctx, id)
}

func (s *Store) ListSpecs(ctx context.Context, f storage.SpecFilter, o storage.ListOptions) (*storage.SpecListResult, error) {
	return s.inner.ListSpecs(ctx, f, o)
}

func (s *Store) DeleteSpec(ctx context.Context, id string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_SpecDelete{SpecDelete: id}})
}

// === Rollouts ===

func (s *Store) UpsertRollout(ctx context.Context, r *model.Rollout) error {
	if r == nil {
		return storage.ErrInvalidArgument
	}
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_RolloutUpsert{RolloutUpsert: wire.RolloutToProto(r)}})
}

func (s *Store) GetRollout(ctx context.Context, id string) (*model.Rollout, error) {
	return s.inner.GetRollout(ctx, id)
}

func (s *Store) ListRollouts(ctx context.Context, f storage.RolloutFilter, o storage.ListOptions) (*storage.RolloutListResult, error) {
	return s.inner.ListRollouts(ctx, f, o)
}

func (s *Store) DeleteRollout(ctx context.Context, id string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_RolloutDelete{RolloutDelete: id}})
}

func (s *Store) DeleteRolloutsBySpec(ctx context.Context, specID string) error {
	return s.applyOp(&raftv1.Op{Op: &raftv1.Op_RolloutDeleteBySpec{RolloutDeleteBySpec: specID}})
}

// === FilterFactory — passthrough ===

func (s *Store) BuildRolloutFilter(c storage.RolloutQueryCriteria) storage.RolloutFilter {
	return s.inner.BuildRolloutFilter(c)
}

func (s *Store) BuildSpecFilter(c storage.SpecQueryCriteria) storage.SpecFilter {
	return s.inner.BuildSpecFilter(c)
}

// === WithTx — recording batch ===

// WithTx runs fn against a recording view that captures every mutation as
// a proto Op. At end of fn the captured ops are submitted to Raft as one
// Command and applied atomically on every replica.
//
// fn's reads within the tx see:
//  1. Values written earlier in the same fn (the overlay), then
//  2. The inner store (followers won't have them, but we are running on the
//     leader where the inner store is the up-to-date one thanks to previous
//     FSM applies).
//
// Contract: only leader can run WithTx. If Raft.State() != Leader the tx is
// rejected before fn executes.
func (s *Store) WithTx(ctx context.Context, fn func(tx storage.Storage) error) error {
	if fn == nil {
		return storage.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.raft.State() != hraft.Leader {
		return fmt.Errorf("raft: WithTx on non-leader replica")
	}

	s.txMu.Lock()
	defer s.txMu.Unlock()

	view := newTxView(s.inner)
	if err := fn(view); err != nil {
		return err
	}
	if len(view.ops) == 0 {
		return nil
	}
	return s.apply(view.ops)
}
