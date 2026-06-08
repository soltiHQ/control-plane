package loghub_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	taskv1 "github.com/soltiHQ/control-plane/api/gen/solti/task/v1"
	"github.com/soltiHQ/control-plane/internal/loghub"
)

func chunk(line string) *taskv1.StreamTaskLogsResponse {
	return &taskv1.StreamTaskLogsResponse{
		Kind: &taskv1.StreamTaskLogsResponse_Chunk{Chunk: &taskv1.OutputChunk{Line: []byte(line)}},
	}
}

// TestSingleAgentConnection — два подписчика на (agent, task) должны
// разделять одно backing-подключение.
func TestSingleAgentConnection(t *testing.T) {
	var opens int
	src := make(chan *taskv1.StreamTaskLogsResponse, 4)

	hub := loghub.New(func(_ context.Context, _, _ string) (<-chan *taskv1.StreamTaskLogsResponse, error) {
		opens++
		return src, nil
	}, loghub.Options{SubscriberBuffer: 4})

	a, unsubA, err := hub.Subscribe(context.Background(), "ag", "t1")
	if err != nil {
		t.Fatal(err)
	}
	defer unsubA()

	b, unsubB, err := hub.Subscribe(context.Background(), "ag", "t1")
	if err != nil {
		t.Fatal(err)
	}
	defer unsubB()

	if opens != 1 {
		t.Fatalf("expected 1 backing connection, got %d", opens)
	}

	src <- chunk("hello")

	for _, ch := range []<-chan *taskv1.StreamTaskLogsResponse{a, b} {
		select {
		case ev := <-ch:
			if string(ev.GetChunk().GetLine()) != "hello" {
				t.Fatalf("unexpected event: %v", ev)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber didn't receive event")
		}
	}
}

// TestSourceClosesOnLastUnsub — после ухода последнего подписчика hub
// освобождает backing-подключение; следующий Subscribe открывает новое.
func TestSourceClosesOnLastUnsub(t *testing.T) {
	var opens int
	src := make(chan *taskv1.StreamTaskLogsResponse)

	hub := loghub.New(func(_ context.Context, _, _ string) (<-chan *taskv1.StreamTaskLogsResponse, error) {
		opens++
		// каждый Subscribe получает свежий канал, чтобы не реюзать закрытый
		src = make(chan *taskv1.StreamTaskLogsResponse)
		return src, nil
	}, loghub.Options{})

	_, unsub, err := hub.Subscribe(context.Background(), "ag", "t")
	if err != nil {
		t.Fatal(err)
	}
	unsub()

	// Даём pump goroutine завершиться.
	time.Sleep(20 * time.Millisecond)

	_, unsub2, err := hub.Subscribe(context.Background(), "ag", "t")
	if err != nil {
		t.Fatal(err)
	}
	defer unsub2()

	if opens != 2 {
		t.Fatalf("expected hub to reopen, opens=%d", opens)
	}
}

// TestSubscribeAfterSourceClosed — если backing-стрим завершился сам по
// себе и source ещё чистится, новый Subscribe либо открывает новый source,
// либо корректно ругается ErrSourceClosed.
func TestSubscribeReopensAfterEOF(t *testing.T) {
	var opens int

	hub := loghub.New(func(_ context.Context, _, _ string) (<-chan *taskv1.StreamTaskLogsResponse, error) {
		opens++
		ch := make(chan *taskv1.StreamTaskLogsResponse)
		close(ch) // EOF сразу
		return ch, nil
	}, loghub.Options{})

	_, unsub, err := hub.Subscribe(context.Background(), "ag", "t")
	if err != nil && !errors.Is(err, loghub.ErrSourceClosed) {
		t.Fatalf("first subscribe: %v", err)
	}
	if unsub != nil {
		unsub()
	}

	time.Sleep(20 * time.Millisecond)

	// Второй Subscribe должен либо реоткрыть, либо тоже сразу EOF — главное
	// не залипнуть и не упасть.
	_, unsub2, err := hub.Subscribe(context.Background(), "ag", "t")
	if err != nil && !errors.Is(err, loghub.ErrSourceClosed) {
		t.Fatalf("second subscribe: %v", err)
	}
	if unsub2 != nil {
		unsub2()
	}
	if opens < 1 {
		t.Fatal("expected at least one open")
	}
}

// TestConcurrentChurnDuringStream stresses broadcast against concurrent
// unsubscribes: many subscribers churn (subscribe → read → unsubscribe) on the
// same key while the backing source streams continuously. On the pre-fix
// broadcast (which released the lock before sending) this panics with
// "send on closed channel"; with the fix it stays clean. Run with -race for
// full coverage of the send/close interleaving.
func TestConcurrentChurnDuringStream(t *testing.T) {
	// open mirrors the real proxy contract: a producer goroutine fires events
	// until ctx is cancelled, then closes the channel.
	open := func(ctx context.Context, _, _ string) (<-chan *taskv1.StreamTaskLogsResponse, error) {
		ch := make(chan *taskv1.StreamTaskLogsResponse)
		go func() {
			defer close(ch)
			for {
				select {
				case <-ctx.Done():
					return
				case ch <- chunk("x"):
				}
			}
		}()
		return ch, nil
	}
	// Buffer 1 → frequent buffer-full drops, exercising both the Lagged path
	// and plain sends under contention.
	hub := loghub.New(open, loghub.Options{SubscriberBuffer: 1})

	var wg sync.WaitGroup
	deadline := time.Now().Add(300 * time.Millisecond)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				ch, unsub, err := hub.Subscribe(context.Background(), "ag", "t")
				if err != nil {
					continue // ErrSourceClosed is a retryable race outcome
				}
				for j := 0; j < 3; j++ {
					select {
					case <-ch:
					default:
					}
				}
				unsub()
			}
		}()
	}
	wg.Wait()
}
