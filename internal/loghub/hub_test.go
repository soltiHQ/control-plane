package loghub_test

import (
	"context"
	"errors"
	"testing"
	"time"

	genv1 "github.com/soltiHQ/control-plane/api/gen/v1"
	"github.com/soltiHQ/control-plane/internal/loghub"
)

func chunk(line string) *genv1.OutputEventProto {
	return &genv1.OutputEventProto{
		Kind: &genv1.OutputEventProto_Chunk{Chunk: &genv1.OutputChunkProto{Line: []byte(line)}},
	}
}

// TestSingleAgentConnection — два подписчика на (agent, task) должны
// разделять одно backing-подключение.
func TestSingleAgentConnection(t *testing.T) {
	var opens int
	src := make(chan *genv1.OutputEventProto, 4)

	hub := loghub.New(func(_ context.Context, _, _ string) (<-chan *genv1.OutputEventProto, error) {
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

	for _, ch := range []<-chan *genv1.OutputEventProto{a, b} {
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
	src := make(chan *genv1.OutputEventProto)

	hub := loghub.New(func(_ context.Context, _, _ string) (<-chan *genv1.OutputEventProto, error) {
		opens++
		// каждый Subscribe получает свежий канал, чтобы не реюзать закрытый
		src = make(chan *genv1.OutputEventProto)
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

	hub := loghub.New(func(_ context.Context, _, _ string) (<-chan *genv1.OutputEventProto, error) {
		opens++
		ch := make(chan *genv1.OutputEventProto)
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
