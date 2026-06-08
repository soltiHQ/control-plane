package inmemory

import (
	"github.com/soltiHQ/control-plane/domain/model"
)

// SnapshotContent is the full state of the Store as flat slices.
//
// It is the unit of data exchanged with the Raft FSM at log-compaction
// time: SnapshotForRaft produces one, RestoreFromSnapshot consumes one.
// Order of elements within each slice is unspecified.
type SnapshotContent struct {
	Agents      []*model.Agent
	AgentCreds  []*model.AgentCredential
	Users       []*model.User
	Roles       []*model.Role
	Credentials []*model.Credential
	Verifiers   []*model.Verifier
	Sessions    []*model.Session
	Specs       []*model.Spec
	Rollouts    []*model.Rollout
}

// SnapshotForRaft returns a consistent point-in-time snapshot of every entity.
//
// Safe to call concurrently with mutations: the underlying maps are
// shallow-copied while held under each GenericStore's read lock, and the
// values are already clones of caller inputs. Callers must not mutate the
// returned objects — they share identity with the store's live entries.
func (s *Store) SnapshotForRaft() SnapshotContent {
	snap := s.takeSnapshot()
	return SnapshotContent{
		Agents:      valuesOf(snap.agents),
		AgentCreds:  valuesOf(snap.agentCreds),
		Users:       valuesOf(snap.users),
		Roles:       valuesOf(snap.roles),
		Credentials: valuesOf(snap.credentials),
		Verifiers:   valuesOf(snap.verifiers),
		Sessions:    valuesOf(snap.sessions),
		Specs:       valuesOf(snap.specs),
		Rollouts:    valuesOf(snap.rollouts),
	}
}

// RestoreFromSnapshot replaces the entire Store contents with c.
//
// Used by the Raft FSM during snapshot-based recovery; must not be called
// during normal operation. Existing data is dropped. Element pointers are
// installed as-is — callers must not retain references after the call.
func (s *Store) RestoreFromSnapshot(c SnapshotContent) {
	s.restoreSnapshot(snapshot{
		agents:      indexByID(c.Agents),
		agentCreds:  indexByID(c.AgentCreds),
		users:       indexByID(c.Users),
		roles:       indexByID(c.Roles),
		credentials: indexByID(c.Credentials),
		verifiers:   indexByID(c.Verifiers),
		sessions:    indexByID(c.Sessions),
		specs:       indexByID(c.Specs),
		rollouts:    indexByID(c.Rollouts),
	})
}

// valuesOf returns the values of m as a slice. Order is unspecified.
func valuesOf[T any](m map[string]T) []T {
	out := make([]T, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// hasID is the minimal interface every domain entity satisfies via ID().
type hasID interface {
	ID() string
}

// indexByID builds an id→entity map for restoreSnapshot input.
func indexByID[T hasID](items []T) map[string]T {
	out := make(map[string]T, len(items))
	for _, v := range items {
		out[v.ID()] = v
	}
	return out
}
