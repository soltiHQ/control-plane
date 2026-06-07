package transportctx

import (
	"context"
	"sync"
	"testing"
)

func TestErrorSlotRoundtrip(t *testing.T) {
	ctx := WithErrorSlot(context.Background())

	if got := TryError(ctx); got != "" {
		t.Fatalf("fresh slot: TryError = %q, want empty", got)
	}
	SetError(ctx, "boom")
	if got := TryError(ctx); got != "boom" {
		t.Fatalf("TryError = %q, want %q", got, "boom")
	}
	SetError(ctx, "later")
	if got := TryError(ctx); got != "later" {
		t.Fatalf("TryError after overwrite = %q, want %q", got, "later")
	}
}

func TestErrorSlotAbsentIsNoop(t *testing.T) {
	ctx := context.Background()
	SetError(ctx, "ignored")
	if got := TryError(ctx); got != "" {
		t.Fatalf("TryError without slot = %q, want empty", got)
	}
}

func TestErrorSlotConcurrent(t *testing.T) {
	ctx := WithErrorSlot(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); SetError(ctx, "x") }()
		go func() { defer wg.Done(); _ = TryError(ctx) }()
	}
	wg.Wait()
}

func TestRequestID(t *testing.T) {
	ctx := WithRequestID(context.Background(), "rid-1")
	if rid, ok := RequestID(ctx); !ok || rid != "rid-1" {
		t.Fatalf("RequestID = (%q, %v), want (rid-1, true)", rid, ok)
	}

	if rid, ok := RequestID(WithRequestID(context.Background(), "")); ok || rid != "" {
		t.Fatalf("empty RequestID = (%q, %v), want (\"\", false)", rid, ok)
	}
	if got := TryRequestID(context.Background()); got != unknownRequestID {
		t.Fatalf("TryRequestID without value = %q, want %q", got, unknownRequestID)
	}
}

func TestIdentityNilGuard(t *testing.T) {
	ctx := WithIdentity(context.Background(), nil)
	if id, ok := Identity(ctx); ok || id != nil {
		t.Fatalf("Identity(nil) = (%v, %v), want (nil, false)", id, ok)
	}
}
