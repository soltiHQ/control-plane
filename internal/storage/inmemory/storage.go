package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// Compile-time checks that Store implements the required interfaces.
var (
	_ storage.Storage         = (*Store)(nil)
	_ storage.AgentStore      = (*Store)(nil)
	_ storage.UserStore       = (*Store)(nil)
	_ storage.CredentialStore = (*Store)(nil)
	_ storage.RoleStore       = (*Store)(nil)
	_ storage.VerifierStore   = (*Store)(nil)
	_ storage.SessionStore    = (*Store)(nil)
	_ storage.SpecStore       = (*Store)(nil)
	_ storage.RolloutStore    = (*Store)(nil)
)

// Store provides an in-memory implementation of storage.Storage using GenericStore.
type Store struct {
	// txMu serialises WithTx calls against each other (one transaction at a time per Store).
	txMu sync.Mutex

	agents      *GenericStore[*model.Agent]
	users       *GenericStore[*model.User]
	roles       *GenericStore[*model.Role]
	credentials *GenericStore[*model.Credential]
	verifiers   *GenericStore[*model.Verifier]
	sessions    *GenericStore[*model.Session]
	specs       *GenericStore[*model.Spec]
	rollouts    *GenericStore[*model.Rollout]
}

// New creates a new in-memory store with an empty state.
func New() *Store {
	return &Store{
		agents:      NewGenericStore[*model.Agent](),
		users:       NewGenericStore[*model.User](),
		roles:       NewGenericStore[*model.Role](),
		credentials: NewGenericStore[*model.Credential](),
		verifiers:   NewGenericStore[*model.Verifier](),
		sessions:    NewGenericStore[*model.Session](),
		specs:       NewGenericStore[*model.Spec](),
		rollouts:    NewGenericStore[*model.Rollout](),
	}
}

// --- Agents ---

func (s *Store) UpsertAgent(ctx context.Context, a *model.Agent) error {
	if a == nil {
		return storage.ErrInvalidArgument
	}
	return s.agents.Upsert(ctx, a)
}

func (s *Store) GetAgent(ctx context.Context, id string) (*model.Agent, error) {
	return s.agents.Get(ctx, id)
}

func (s *Store) ListAgents(ctx context.Context, filter storage.AgentFilter, opts storage.ListOptions) (*storage.AgentListResult, error) {
	var predicate func(*model.Agent) bool

	if filter != nil {
		f, ok := filter.(*AgentFilter)
		if !ok {
			return nil, storage.ErrInvalidArgument
		}
		predicate = f.Matches
	}
	return s.agents.List(ctx, predicate, opts)
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	return s.agents.Delete(ctx, id)
}

// --- Users ---

func (s *Store) UpsertUser(ctx context.Context, u *model.User) error {
	if u == nil {
		return storage.ErrInvalidArgument
	}
	return s.users.Upsert(ctx, u)
}

func (s *Store) GetUser(ctx context.Context, id string) (*model.User, error) {
	return s.users.Get(ctx, id)
}

func (s *Store) GetUserBySubject(ctx context.Context, subject string) (*model.User, error) {
	if subject == "" {
		return nil, storage.ErrInvalidArgument
	}

	s.users.mu.RLock()
	defer s.users.mu.RUnlock()

	var (
		found *model.User
		i     int
	)
	for _, u := range s.users.data {
		if i%1000 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		i++

		if u.Subject() != subject {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%w: non-unique user subject %q", storage.ErrInternal, subject)
		}
		found = u
	}
	if found == nil {
		return nil, storage.ErrNotFound
	}
	return found.Clone(), nil
}

func (s *Store) ListUsers(ctx context.Context, filter storage.UserFilter, opts storage.ListOptions) (*storage.UserListResult, error) {
	var predicate func(*model.User) bool

	if filter != nil {
		f, ok := filter.(*UserFilter)
		if !ok {
			return nil, storage.ErrInvalidArgument
		}
		predicate = f.Matches
	}

	return s.users.List(ctx, predicate, opts)
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	return s.users.Delete(ctx, id)
}

// --- Credentials ---

func (s *Store) UpsertCredential(ctx context.Context, c *model.Credential) error {
	if c == nil {
		return storage.ErrInvalidArgument
	}
	return s.credentials.Upsert(ctx, c)
}

func (s *Store) GetCredential(ctx context.Context, id string) (*model.Credential, error) {
	return s.credentials.Get(ctx, id)
}

func (s *Store) GetCredentialByUserAndAuth(ctx context.Context, userID string, auth enum.Auth) (*model.Credential, error) {
	if userID == "" {
		return nil, storage.ErrInvalidArgument
	}

	s.credentials.mu.RLock()
	defer s.credentials.mu.RUnlock()

	var (
		found *model.Credential
		i     int
	)
	for _, c := range s.credentials.data {
		if i%1000 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		i++

		if c.UserID() != userID || c.AuthKind() != auth {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%w: non-unique credential for user %q auth %q", storage.ErrInternal, userID, auth)
		}
		found = c
	}
	if found == nil {
		return nil, storage.ErrNotFound
	}
	return found.Clone(), nil
}

func (s *Store) ListCredentialsByUser(ctx context.Context, userID string) ([]*model.Credential, error) {
	if userID == "" {
		return nil, storage.ErrInvalidArgument
	}

	res, err := s.credentials.List(ctx, func(c *model.Credential) bool {
		return c.UserID() == userID
	}, storage.ListOptions{Limit: storage.MaxListLimit})
	if err != nil {
		return nil, err
	}

	return res.Items, nil
}

func (s *Store) DeleteCredential(ctx context.Context, id string) error {
	return s.credentials.Delete(ctx, id)
}

// --- Verifiers ---

func (s *Store) UpsertVerifier(ctx context.Context, v *model.Verifier) error {
	if v == nil {
		return storage.ErrInvalidArgument
	}
	return s.verifiers.Upsert(ctx, v)
}

func (s *Store) GetVerifier(ctx context.Context, id string) (*model.Verifier, error) {
	return s.verifiers.Get(ctx, id)
}

func (s *Store) GetVerifierByCredential(ctx context.Context, credentialID string) (*model.Verifier, error) {
	if credentialID == "" {
		return nil, storage.ErrInvalidArgument
	}

	s.verifiers.mu.RLock()
	defer s.verifiers.mu.RUnlock()

	var (
		found *model.Verifier
		i     int
	)
	for _, v := range s.verifiers.data {
		if i%1000 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		i++

		if v.CredentialID() != credentialID {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%w: non-unique verifier for credential %q", storage.ErrInternal, credentialID)
		}
		found = v
	}
	if found == nil {
		return nil, storage.ErrNotFound
	}
	return found.Clone(), nil
}

func (s *Store) DeleteVerifier(ctx context.Context, id string) error {
	return s.verifiers.Delete(ctx, id)
}

func (s *Store) DeleteVerifierByCredential(ctx context.Context, credentialID string) error {
	if credentialID == "" {
		return storage.ErrInvalidArgument
	}

	s.verifiers.mu.RLock()
	var (
		ids = make([]string, 0, 1)
		i   = 0
	)
	for id, v := range s.verifiers.data {
		if i%1000 == 0 {
			select {
			case <-ctx.Done():
				s.verifiers.mu.RUnlock()
				return ctx.Err()
			default:
			}
		}
		i++

		if v.CredentialID() == credentialID {
			ids = append(ids, id)
		}
	}
	s.verifiers.mu.RUnlock()

	for _, id := range ids {
		_ = s.verifiers.Delete(ctx, id)
	}
	return nil
}

// --- Sessions ---

func (s *Store) CreateSession(ctx context.Context, sess *model.Session) error {
	if sess == nil {
		return storage.ErrInvalidArgument
	}
	return s.sessions.Create(ctx, sess)
}

func (s *Store) GetSession(ctx context.Context, id string) (*model.Session, error) {
	return s.sessions.Get(ctx, id)
}

func (s *Store) ListSessionsByUser(ctx context.Context, userID string) ([]*model.Session, error) {
	if userID == "" {
		return nil, storage.ErrInvalidArgument
	}

	res, err := s.sessions.List(ctx, func(sess *model.Session) bool {
		return sess.UserID() == userID
	}, storage.ListOptions{Limit: storage.MaxListLimit})
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

func (s *Store) RotateRefresh(ctx context.Context, sessionID string, newHash []byte, newExpiresAt time.Time) error {
	if sessionID == "" || len(newHash) == 0 || newExpiresAt.IsZero() {
		return storage.ErrInvalidArgument
	}
	return s.sessions.Update(ctx, sessionID, func(cur *model.Session) (*model.Session, error) {
		if err := cur.SetRefreshHash(newHash); err != nil {
			return nil, err
		}
		if err := cur.SetExpiresAt(newExpiresAt); err != nil {
			return nil, err
		}
		return cur, nil
	})
}

func (s *Store) RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) error {
	if sessionID == "" || revokedAt.IsZero() {
		return storage.ErrInvalidArgument
	}
	return s.sessions.Update(ctx, sessionID, func(cur *model.Session) (*model.Session, error) {
		if err := cur.Revoke(revokedAt); err != nil {
			return nil, err
		}
		return cur, nil
	})
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return s.sessions.Delete(ctx, id)
}

func (s *Store) DeleteSessionsByUser(ctx context.Context, userID string) error {
	if userID == "" {
		return storage.ErrInvalidArgument
	}

	sessions, err := s.ListSessionsByUser(ctx, userID)
	if err != nil {
		return err
	}

	for _, sess := range sessions {
		if sess == nil {
			continue
		}
		_ = s.sessions.Delete(ctx, sess.ID())
	}
	return nil
}

// --- Roles ---

func (s *Store) UpsertRole(ctx context.Context, r *model.Role) error {
	if r == nil {
		return storage.ErrInvalidArgument
	}
	return s.roles.Upsert(ctx, r)
}

func (s *Store) GetRole(ctx context.Context, id string) (*model.Role, error) {
	return s.roles.Get(ctx, id)
}

func (s *Store) GetRoles(ctx context.Context, ids []string) ([]*model.Role, error) {
	if len(ids) == 0 {
		return nil, storage.ErrInvalidArgument
	}
	for _, id := range ids {
		if id == "" {
			return nil, storage.ErrInvalidArgument
		}
	}

	var (
		seen   = make(map[string]struct{}, len(ids))
		unique = make([]string, 0, len(ids))
	)
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	roles, err := s.roles.GetMany(ctx, unique)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*model.Role, len(roles))
	for _, r := range roles {
		byID[r.ID()] = r
	}
	out := make([]*model.Role, 0, len(ids))
	for _, id := range ids {
		r, ok := byID[id]
		if !ok {
			// Should not happen if GetMany succeeded, but keep it strict.
			return nil, storage.ErrInternal
		}
		out = append(out, r.Clone())
	}
	return out, nil
}

func (s *Store) GetRoleByName(ctx context.Context, name string) (*model.Role, error) {
	if name == "" {
		return nil, storage.ErrInvalidArgument
	}

	s.roles.mu.RLock()
	defer s.roles.mu.RUnlock()

	var (
		found *model.Role
		i     int
	)
	for _, r := range s.roles.data {
		if i%1000 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		i++

		if r.Name() != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%w: non-unique role name %q", storage.ErrInternal, name)
		}
		found = r
	}

	if found == nil {
		return nil, storage.ErrNotFound
	}
	return found.Clone(), nil
}

func (s *Store) ListRoles(ctx context.Context, filter storage.RoleFilter, opts storage.ListOptions) (*storage.RoleListResult, error) {
	var predicate func(*model.Role) bool

	if filter != nil {
		f, ok := filter.(*RoleFilter)
		if !ok {
			return nil, storage.ErrInvalidArgument
		}
		predicate = f.Matches
	}
	return s.roles.List(ctx, predicate, opts)
}

func (s *Store) DeleteRole(ctx context.Context, id string) error {
	return s.roles.Delete(ctx, id)
}

// --- Specs ---

func (s *Store) UpsertSpec(ctx context.Context, ts *model.Spec) error {
	if ts == nil {
		return storage.ErrInvalidArgument
	}
	return s.specs.Upsert(ctx, ts)
}

func (s *Store) GetSpec(ctx context.Context, id string) (*model.Spec, error) {
	return s.specs.Get(ctx, id)
}

func (s *Store) ListSpecs(ctx context.Context, filter storage.SpecFilter, opts storage.ListOptions) (*storage.SpecListResult, error) {
	var predicate func(*model.Spec) bool

	if filter != nil {
		f, ok := filter.(*SpecFilter)
		if !ok {
			return nil, storage.ErrInvalidArgument
		}
		predicate = f.Matches
	}
	return s.specs.List(ctx, predicate, opts)
}

func (s *Store) DeleteSpec(ctx context.Context, id string) error {
	return s.specs.Delete(ctx, id)
}

// --- Rollouts ---

func (s *Store) UpsertRollout(ctx context.Context, ss *model.Rollout) error {
	if ss == nil {
		return storage.ErrInvalidArgument
	}
	return s.rollouts.Upsert(ctx, ss)
}

func (s *Store) GetRollout(ctx context.Context, id string) (*model.Rollout, error) {
	return s.rollouts.Get(ctx, id)
}

func (s *Store) ListRollouts(ctx context.Context, filter storage.RolloutFilter, opts storage.ListOptions) (*storage.RolloutListResult, error) {
	var predicate func(*model.Rollout) bool

	if filter != nil {
		f, ok := filter.(*RolloutFilter)
		if !ok {
			return nil, storage.ErrInvalidArgument
		}
		predicate = f.Matches
	}
	return s.rollouts.List(ctx, predicate, opts)
}

func (s *Store) DeleteRollout(ctx context.Context, id string) error {
	return s.rollouts.Delete(ctx, id)
}

func (s *Store) DeleteRolloutsBySpec(ctx context.Context, specID string) error {
	if specID == "" {
		return storage.ErrInvalidArgument
	}

	s.rollouts.mu.RLock()
	ids := make([]string, 0)
	for id, ss := range s.rollouts.data {
		if ss.SpecID() == specID {
			ids = append(ids, id)
		}
	}
	s.rollouts.mu.RUnlock()

	for _, id := range ids {
		_ = s.rollouts.Delete(ctx, id)
	}
	return nil
}

// --- FilterFactory ---

func (s *Store) BuildRolloutFilter(c storage.RolloutQueryCriteria) storage.RolloutFilter {
	f := NewRolloutFilter()
	if c.SpecID != "" {
		f.BySpecID(c.SpecID)
	}
	if c.AgentID != "" {
		f.ByAgentID(c.AgentID)
	}
	if len(c.Statuses) > 0 {
		f.ByStatuses(c.Statuses...)
	}
	return f
}

func (s *Store) BuildSpecFilter(c storage.SpecQueryCriteria) storage.SpecFilter {
	f := NewSpecFilter()
	if c.Query != "" {
		f.Query(c.Query)
	}
	return f
}
