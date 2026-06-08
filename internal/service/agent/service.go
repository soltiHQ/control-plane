package agent

import (
	"context"
	"errors"

	"github.com/rs/zerolog"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/service"
	"github.com/soltiHQ/control-plane/internal/storage"
)

// Service provides agent management operations.
type Service struct {
	logger zerolog.Logger
	store  storage.AgentStore
}

// New creates a new agent service.
func New(store storage.AgentStore, logger zerolog.Logger) *Service {
	if store == nil {
		panic("agent.Service: store is nil")
	}
	return &Service{
		logger: logger.With().Str("service", "agents").Logger(),
		store:  store,
	}
}

// List returns a page of agents matching the query.
func (s *Service) List(ctx context.Context, q ListQuery) (*Page, error) {
	res, err := s.store.ListAgents(ctx, q.Filter, storage.ListOptions{
		Limit:  service.NormalizeListLimit(q.Limit, defaultListLimit),
		Cursor: q.Cursor,
	})
	if err != nil {
		return nil, err
	}

	out := make([]*model.Agent, 0, len(res.Items))
	for _, a := range res.Items {
		if a == nil {
			continue
		}
		out = append(out, a.Clone())
	}
	return &Page{
		Items:      out,
		NextCursor: res.NextCursor,
	}, nil
}

// Get returns a single agent by ID.
func (s *Service) Get(ctx context.Context, id string) (*model.Agent, error) {
	if id == "" {
		return nil, storage.ErrInvalidArgument
	}

	a, err := s.store.GetAgent(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, storage.ErrInternal
	}
	return a.Clone(), nil
}

// Upsert an agent.
//
// If the agent already exists, control-plane owned labels and the original createdAt timestamp are preserved
// because they are not part of the discovery payload reported by the agent.
func (s *Service) Upsert(ctx context.Context, m *model.Agent) error {
	var existed bool
	existing, err := s.store.GetAgent(ctx, m.ID())
	switch {
	case err == nil:
		existed = true
		m.SetCreatedAt(existing.CreatedAt())
		for k, v := range existing.LabelsAll() {
			m.LabelAdd(k, v)
		}
		if m.HeartbeatInterval() == 0 && existing.HeartbeatInterval() > 0 {
			m.SetHeartbeatInterval(existing.HeartbeatInterval())
		}
	case errors.Is(err, storage.ErrNotFound):
	default:
		return err
	}
	if hb := m.HeartbeatInterval(); hb > 0 {
		m.SetStaleAt(m.LastSeenAt().Add(hb))
	}
	if err = s.store.UpsertAgent(ctx, m); err != nil {
		return err
	}

	s.logger.Debug().
		Str("agent_id", m.ID()).
		Bool("existed", existed).
		Str("heartbeat", m.HeartbeatInterval().String()).
		Msg("agent upserted")
	return nil
}

// PatchLabels replaces labels for an agent.
func (s *Service) PatchLabels(ctx context.Context, req PatchLabels) (*model.Agent, error) {
	if req.ID == "" {
		return nil, storage.ErrInvalidArgument
	}

	agent, err := s.store.GetAgent(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, storage.ErrInternal
	}

	before := agent.UpdatedAt()
	replaceLabels(agent, req.Labels)
	if agent.UpdatedAt().Equal(before) {
		// No label actually changed — skip the write so we don't emit a
		// redundant Raft entry and SSE notification for a no-op patch.
		return agent.Clone(), nil
	}
	if err = s.store.UpsertAgent(ctx, agent); err != nil {
		return nil, err
	}

	s.logger.Debug().
		Str("agent_id", req.ID).
		Int("labels", len(req.Labels)).
		Msg("labels patched")
	return agent.Clone(), nil
}

// replaceLabels reconciles the agent's labels toward the desired set as a diff (not delete-all + add-all),
// re-applying the same labels is a true no-op and leaves UpdatedAt untouched.
// An empty value means "remove the key".
func replaceLabels(a *model.Agent, labels map[string]string) {
	for k := range a.LabelsAll() {
		if v, ok := labels[k]; !ok || v == "" {
			a.LabelDelete(k)
		}
	}
	for k, v := range labels {
		if k == "" || v == "" {
			continue
		}
		a.LabelAdd(k, v)
	}
}
