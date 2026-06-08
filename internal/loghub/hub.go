// Package loghub fans out a single backing log stream (CP → agent) to
// N UI subscribers. The Hub maintains one underlying connection per
// (agentID, taskID) and closes it when the last subscriber leaves.
//
// Slow-subscriber policy: per-subscriber buffer; on overflow events are
// dropped and a Lagged sentinel is enqueued so the client can render
// "N events skipped" without falling further behind the source.
package loghub

import (
	"context"
	"errors"
	"sync"

	taskv1 "github.com/soltiHQ/control-plane/api/gen/solti/task/v1"
)

// ErrSourceClosed is returned by Subscribe when the underlying source has
// already finished (e.g. the agent stream ended between the lookup and the
// subscriber registration). Callers should treat it like a brief, retryable
// "not available" — usually a fresh Subscribe will reopen.
var ErrSourceClosed = errors.New("loghub: source closed")

// OpenFunc opens a backing log stream for a given (agentID, taskID).
// The returned channel must be closed by the producer when the stream ends.
type OpenFunc func(ctx context.Context, agentID, taskID string) (<-chan *taskv1.StreamTaskLogsResponse, error)

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
// Slow readers receive a synthetic Lagged event when they fall behind
// the source, so they can render the gap without polluting later events.
//
// Racing a teardown: if the backing source is ending concurrently, Subscribe
// either returns ErrSourceClosed (the source was already torn down) or, more
// rarely, a valid channel that the Hub closes immediately with no events
// delivered. Callers should treat both as "retry": a fresh Subscribe reopens
// the source.
func (h *Hub) Subscribe(ctx context.Context, agentID, taskID string) (<-chan *taskv1.StreamTaskLogsResponse, func(), error) {
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
	ch      chan *taskv1.StreamTaskLogsResponse
	dropped uint64 // pending Lagged count; guarded by source.mu
}

func (s *source) addSub(buffer int) (<-chan *taskv1.StreamTaskLogsResponse, func(), error) {
	sb := &sub{ch: make(chan *taskv1.StreamTaskLogsResponse, buffer)}

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
func (s *source) pump(stream <-chan *taskv1.StreamTaskLogsResponse) {
	defer s.shutdown()
	for ev := range stream {
		s.broadcast(ev)
	}
}

// broadcast sends ev to every subscriber, holding s.mu for the whole fan-out.
//
// The lock is held during the sends — not just to snapshot the set — on
// purpose: removeSub/shutdown close subscriber channels under s.mu, so sending
// under the same lock makes "send" and "close" mutually exclusive. Releasing
// the lock before sending (the obvious optimization) races a concurrent
// unsubscribe and panics with "send on closed channel" — a closed channel is
// "ready" for send inside a select, so the default case does not save us.
// Holding the lock is cheap because every send is non-blocking.
//
// If a subscriber's buffer is full the event is dropped and a pending Lagged
// count is incremented; the next successful send injects a Lagged envelope
// first so the client can render the gap.
func (s *source) broadcast(ev *taskv1.StreamTaskLogsResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for sb := range s.subs {
		if sb.dropped > 0 {
			lagged := &taskv1.StreamTaskLogsResponse{
				Kind: &taskv1.StreamTaskLogsResponse_Lagged{Lagged: &taskv1.Lagged{Skipped: sb.dropped}},
			}
			select {
			case sb.ch <- lagged:
				sb.dropped = 0
			default:
				sb.dropped++
				continue
			}
		}
		select {
		case sb.ch <- ev:
		default:
			sb.dropped++
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
