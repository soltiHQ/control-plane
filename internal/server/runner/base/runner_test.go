package base_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/soltiHQ/control-plane/internal/cluster/standalone"
	"github.com/soltiHQ/control-plane/internal/server/runner/base"
)

func discardLogger() zerolog.Logger { return zerolog.New(io.Discard) }

func TestTicksWhileLeaderAndStopDrains(t *testing.T) {
	var ticks atomic.Int64
	r := base.New("test", time.Millisecond, standalone.NewLeadership(), discardLogger(),
		func(context.Context) { ticks.Add(1) })

	errCh := make(chan error, 1)
	go func() { errCh <- r.Start(context.Background()) }()

	deadline := time.After(2 * time.Second)
	for ticks.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("no tick observed while leader")
		case <-time.After(time.Millisecond):
		}
	}

	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not return after Stop drained")
	}
}

func TestStopHonorsDeadlineWhenTickStuck(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var startOnce sync.Once
	r := base.New("test", time.Millisecond, standalone.NewLeadership(), discardLogger(),
		func(context.Context) {
			startOnce.Do(func() { close(started) })
			<-release
		})

	errCh := make(chan error, 1)
	go func() { errCh <- r.Start(context.Background()) }()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := r.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop: want DeadlineExceeded, got %v", err)
	}

	close(release)
	select {
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after release")
	}
}

func TestStopIdempotent(t *testing.T) {
	r := base.New("test", time.Millisecond, standalone.NewLeadership(), discardLogger(),
		func(context.Context) {})

	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}

	go func() { _ = r.Start(context.Background()) }()
	time.Sleep(10 * time.Millisecond)

	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := r.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestStartTwice(t *testing.T) {
	r := base.New("test", time.Second, standalone.NewLeadership(), discardLogger(),
		func(context.Context) {})

	go func() { _ = r.Start(context.Background()) }()
	time.Sleep(10 * time.Millisecond)

	if err := r.Start(context.Background()); !errors.Is(err, base.ErrAlreadyStarted) {
		t.Fatalf("second Start: want ErrAlreadyStarted, got %v", err)
	}
	_ = r.Stop(context.Background())
}
