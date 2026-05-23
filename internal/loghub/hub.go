// Package loghub fans out a single backing log stream (CP → agent) to
// N UI subscribers. The Hub maintains one underlying connection per
// (agentID, taskID) and closes it when the last subscriber leaves.
//
// Slow-subscriber policy: per-subscriber buffer; on overflow events are
// dropped and a LaggedProto sentinel is enqueued so the client can render
// "N events skipped" without falling further behind the source.
package loghub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	genv1 "github.com/soltiHQ/control-plane/api/gen/v1"
)

// ErrSourceClosed is returned by Subscribe when the underlying source has
// already finished (e.g. the agent stream ended between the lookup and the
// subscriber registration). Callers should treat it like a brief, retryable
// "not available" — usually a fresh Subscribe will reopen.
var ErrSourceClosed = errors.New("loghub: source closed")

// OpenFunc opens a backing log stream for a given (agentID, taskID).
// The returned channel must be closed by the producer when the stream ends.
type OpenFunc func(ctx context.Context, agentID, taskID string) (<-chan *genv1.OutputEventProto, error)

// Options tunes Hub behaviour. Zero values use safe defaults.
type Options struct {
	// SubscriberBuffer is the per-subscriber channel buffer size.
	// Larger = more tolerance for slow readers before Lagged kicks in.
	// Default: 64.
	SubscriberBuffer int
}

// Hub multiplexes one backing log stream per (agentID, taskID) over N
// subscribers.
type Hub struct {
	open OpenFunc
	opts Options

	mu      sync.Mutex
	sources map[key]*source
}

type key struct {
	agentID string
	taskID  string
}

// New builds a Hub. open is the factory that produces a backing stream
// when no source for the given (agentID, taskID) exists.
func New(open OpenFunc, opts Options) *Hub {
	if opts.SubscriberBuffer <= 0 {
		opts.SubscriberBuffer = 64
	}
	return &Hub{
		open:    open,
		opts:    opts,
		sources: make(map[key]*source),
	}
}

// Subscribe returns a channel of events plus an unsubscribe function. The
// channel is closed by the Hub when either:
//
//   - The caller invokes unsubscribe (typically via defer), AND no other
//     subscribers remain — in that case the backing stream is also torn down.
//   - The backing agent stream ends naturally — all subscribers' channels
//     close together.
//
// Slow readers receive a synthetic LaggedProto event when they fall behind
// the source, so they can render the gap without polluting later events.
func (h *Hub) Subscribe(ctx context.Context, agentID, taskID string) (<-chan *genv1.OutputEventProto, func(), error) {
	k := key{agentID: agentID, taskID: taskID}

	h.mu.Lock()
	src, ok := h.sources[k]
	if !ok {
		s, err := h.startSource(ctx, k)
		if err != nil {
			h.mu.Unlock()
			return nil, nil, err
		}
		h.sources[k] = s
		src = s
	}
	h.mu.Unlock()

	return src.addSub(h.opts.SubscriberBuffer)
}

func (h *Hub) startSource(parentCtx context.Context, k key) (*source, error) {
	// Source lives independently of the first subscriber's ctx — its
	// lifetime is bounded by len(subs) > 0, not by who happened to open
	// it. Use Background so a subscriber unsubscribing doesn't drop the
	// agent connection underneath any remaining subscribers.
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := h.open(ctx, k.agentID, k.taskID)
	if err != nil {
		cancel()
		return nil, err
	}
	s := &source{
		hub:    h,
		key:    k,
		cancel: cancel,
		subs:   make(map[*sub]struct{}),
	}
	go s.pump(stream)
	return s, nil
}

// source represents one backing log stream + its subscribers.
type source struct {
	hub    *Hub
	key    key
	cancel context.CancelFunc

	mu     sync.Mutex
	subs   map[*sub]struct{}
	closed bool
}

type sub struct {
	ch      chan *genv1.OutputEventProto
	dropped uint64 // atomic; pending Lagged count
}

func (s *source) addSub(buffer int) (<-chan *genv1.OutputEventProto, func(), error) {
	sb := &sub{ch: make(chan *genv1.OutputEventProto, buffer)}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, nil, ErrSourceClosed
	}
	s.subs[sb] = struct{}{}
	s.mu.Unlock()

	return sb.ch, func() { s.removeSub(sb) }, nil
}

func (s *source) removeSub(sb *sub) {
	s.mu.Lock()
	if _, ok := s.subs[sb]; !ok {
		s.mu.Unlock()
		return
	}
	delete(s.subs, sb)
	close(sb.ch)
	last := len(s.subs) == 0
	s.mu.Unlock()

	if last {
		s.shutdown()
	}
}

// pump reads the backing stream and broadcasts to subscribers until the
// stream closes (EOF or cancellation). Final shutdown closes any remaining
// subscriber channels.
func (s *source) pump(stream <-chan *genv1.OutputEventProto) {
	defer s.shutdown()
	for ev := range stream {
		s.broadcast(ev)
	}
}

// broadcast sends ev to every subscriber. Non-blocking — if a subscriber's
// buffer is full, the event is dropped and a pending Lagged count is
// incremented. The next successful send on that subscriber injects a
// LaggedProto envelope first so the client can render the gap.
func (s *source) broadcast(ev *genv1.OutputEventProto) {
	s.mu.Lock()
	subs := make([]*sub, 0, len(s.subs))
	for sb := range s.subs {
		subs = append(subs, sb)
	}
	s.mu.Unlock()

	for _, sb := range subs {
		// If the subscriber has a pending lagged-count from earlier
		// drops, try to flush a Lagged event first. If even that doesn't
		// fit, keep the count for later.
		if dropped := atomic.LoadUint64(&sb.dropped); dropped > 0 {
			lagged := &genv1.OutputEventProto{
				Kind: &genv1.OutputEventProto_Lagged{Lagged: &genv1.LaggedProto{Skipped: dropped}},
			}
			select {
			case sb.ch <- lagged:
				atomic.StoreUint64(&sb.dropped, 0)
			default:
				atomic.AddUint64(&sb.dropped, 1)
				continue
			}
		}
		select {
		case sb.ch <- ev:
		default:
			atomic.AddUint64(&sb.dropped, 1)
		}
	}
}

// shutdown cancels the backing ctx (closing the source's channel), removes
// the source from the Hub's index, and closes any subscriber channels that
// the caller didn't already release. Idempotent.
func (s *source) shutdown() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	subs := make([]*sub, 0, len(s.subs))
	for sb := range s.subs {
		subs = append(subs, sb)
	}
	s.subs = nil
	s.mu.Unlock()

	s.cancel()

	s.hub.mu.Lock()
	if cur, ok := s.hub.sources[s.key]; ok && cur == s {
		delete(s.hub.sources, s.key)
	}
	s.hub.mu.Unlock()

	for _, sb := range subs {
		close(sb.ch)
	}
}
