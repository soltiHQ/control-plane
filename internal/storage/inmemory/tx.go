package inmemory

import (
	"context"

	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// snapshot is a shallow copy of every GenericStore's data map. Values are
// already stored as clones by the write path, so a map-level copy is
// sufficient for rollback.
type snapshot struct {
	agents      map[string]*model.Agent
	users       map[string]*model.User
	roles       map[string]*model.Role
	credentials map[string]*model.Credential
	verifiers   map[string]*model.Verifier
	sessions    map[string]*model.Session
	specs       map[string]*model.Spec
	rollouts    map[string]*model.Rollout
}

func (s *Store) takeSnapshot() snapshot {
	return snapshot{
		agents:      copyMap(s.agents),
		users:       copyMap(s.users),
		roles:       copyMap(s.roles),
		credentials: copyMap(s.credentials),
		verifiers:   copyMap(s.verifiers),
		sessions:    copyMap(s.sessions),
		specs:       copyMap(s.specs),
		rollouts:    copyMap(s.rollouts),
	}
}

func (s *Store) restoreSnapshot(snap snapshot) {
	restoreMap(s.agents, snap.agents)
	restoreMap(s.users, snap.users)
	restoreMap(s.roles, snap.roles)
	restoreMap(s.credentials, snap.credentials)
	restoreMap(s.verifiers, snap.verifiers)
	restoreMap(s.sessions, snap.sessions)
	restoreMap(s.specs, snap.specs)
	restoreMap(s.rollouts, snap.rollouts)
}

func copyMap[T domain.Entity[T]](g *GenericStore[T]) map[string]T {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make(map[string]T, len(g.data))
	for k, v := range g.data {
		out[k] = v
	}
	return out
}

func restoreMap[T domain.Entity[T]](g *GenericStore[T], snap map[string]T) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.data = snap
}

// WithTx serialises transactions and runs fn as an atomic unit: on nil it
// commits (the in-memory Store mutates directly), on error it restores the
// pre-transaction snapshot.
//
// Isolation contract — IMPORTANT:
//
// txMu serialises WithTx calls against EACH OTHER, but NOT against direct
// (non-WithTx) writes such as UpsertAgent. Rollback restores a whole-map
// snapshot, so a direct write that commits while a transaction is in flight is
// LOST if that transaction then fails.
//
// This is safe whenever all writes are serialised by the caller:
//   - Raft mode: every write is applied by the single FSM goroutine (see
//     internal/raft/fsm.go), so direct writes never overlap a transaction.
//   - Standalone mode: concurrent direct writes CAN race a failing transaction.
//     Full standalone isolation needs a store-wide write lock held across fn
//     (a gated/ungated split, since Go locks are not reentrant) — tracked
//     separately.
//
// Nested WithTx inside fn runs fn without re-locking (already-inside-a-tx).
func (s *Store) WithTx(ctx context.Context, fn func(tx storage.Storage) error) error {
	if fn == nil {
		return storage.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.txMu.Lock()
	defer s.txMu.Unlock()

	snap := s.takeSnapshot()
	if err := fn(&txView{Store: s}); err != nil {
		s.restoreSnapshot(snap)
		return err
	}
	return nil
}

// txView wraps *Store so nested WithTx calls inside fn don't re-lock.
type txView struct{ *Store }

func (t *txView) WithTx(ctx context.Context, fn func(tx storage.Storage) error) error {
	if fn == nil {
		return storage.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(t)
}
