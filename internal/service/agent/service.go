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
	store  storage.Storage
}

// New creates a new agent service.
func New(store storage.Storage, logger zerolog.Logger) *Service {
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

	return &Page{
		Items:      res.Items,
		NextCursor: res.NextCursor,
	}, nil
}

// Get returns a single agent by ID.
func (s *Service) Get(ctx context.Context, id string) (*model.Agent, error) {
	if id == "" {
		return nil, storage.ErrInvalidArgument
	}
	return s.store.GetAgent(ctx, id)
}

// Upsert an agent.
func (s *Service) Upsert(ctx context.Context, m *model.Agent) error {
	if m == nil {
		return storage.ErrInvalidArgument
	}

	var existed bool
	err := s.store.WithTx(ctx, func(tx storage.Storage) error {
		existing, err := tx.GetAgent(ctx, m.ID())
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
		return tx.UpsertAgent(ctx, m)
	})
	if err != nil {
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

	var (
		result  *model.Agent
		changed bool
	)
	err := s.store.WithTx(ctx, func(tx storage.Storage) error {
		agent, err := tx.GetAgent(ctx, req.ID)
		if err != nil {
			return err
		}

		before := agent.UpdatedAt()
		replaceLabels(agent, req.Labels)
		if !agent.UpdatedAt().Equal(before) {
			if err = tx.UpsertAgent(ctx, agent); err != nil {
				return err
			}
			changed = true
		}
		result = agent.Clone()
		return nil
	})
	if err != nil {
		return nil, err
	}

	if changed {
		s.logger.Debug().
			Str("agent_id", req.ID).
			Int("labels", len(req.Labels)).
			Msg("labels patched")
	}
	return result, nil
}

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
