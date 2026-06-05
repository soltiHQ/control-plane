package model

import (
	"testing"

	"github.com/soltiHQ/control-plane/domain/enum"
)

// NewSpec starts at generation=1, matching version=1. Both counters are
// 1 out of the gate — only Upsert drifts them apart (metadata edits bump
// only version; runtime edits bump both).
func TestNewSpecDefaultsVersionAndGeneration(t *testing.T) {
	ts, err := NewSpec("sp-1", "demo", "slot-1")
	if err != nil {
		t.Fatalf("NewSpec: %v", err)
	}
	if got, want := ts.Version(), 1; got != want {
		t.Errorf("version: got %d, want %d", got, want)
	}
	if got, want := ts.Generation(), 1; got != want {
		t.Errorf("generation: got %d, want %d", got, want)
	}
	if ts.DeletionRequested() {
		t.Errorf("DeletionRequested should be false on fresh spec")
	}
}

// BumpGeneration and IncrementVersion move independently. This is the
// core invariant behind the two-counter design: service-layer Upsert
// always bumps version, and only bumps generation when RuntimeEquals
// reports false.
func TestVersionAndGenerationAreIndependent(t *testing.T) {
	ts, _ := NewSpec("sp-1", "demo", "slot-1")
	ts.IncrementVersion()
	ts.IncrementVersion()
	ts.BumpGeneration()
	if got, want := ts.Version(), 3; got != want {
		t.Errorf("version: got %d, want %d", got, want)
	}
	if got, want := ts.Generation(), 2; got != want {
		t.Errorf("generation: got %d, want %d", got, want)
	}
}

// MarkForDeletion flips the soft-delete flag. The service keeps the
// record alive until the sync runner finalizer drops it.
func TestMarkForDeletionFlipsFlag(t *testing.T) {
	ts, _ := NewSpec("sp-1", "demo", "slot-1")
	if ts.DeletionRequested() {
		t.Fatal("precondition: DeletionRequested should be false")
	}
	ts.MarkForDeletion()
	if !ts.DeletionRequested() {
		t.Fatalf("MarkForDeletion should set DeletionRequested=true")
	}
}

// Clone carries every new field over. Service layer clones on Get/List,
// so a leak here would silently desynchronise the live object from its
// stored copy.
func TestCloneCopiesNewFields(t *testing.T) {
	ts, _ := NewSpec("sp-1", "demo", "slot-1")
	ts.BumpGeneration()
	ts.MarkForDeletion()

	cp := ts.Clone()

	// Mutate original — clone must not observe these changes.
	ts.IncrementVersion()
	ts.BumpGeneration()

	if cp.Generation() != 2 {
		t.Errorf("clone.Generation leaked: got %d, want 2", cp.Generation())
	}
	if !cp.DeletionRequested() {
		t.Errorf("clone.DeletionRequested should be true")
	}
	if cp.Version() != 1 {
		t.Errorf("clone.Version leaked: got %d, want 1", cp.Version())
	}
}

// RuntimeEquals is what the service uses to decide whether an Upsert
// warrants a generation bump. It compares runtime fields only — name,
// targets, timestamps, version/generation values themselves are ignored.
func TestRuntimeEqualsIgnoresMetadata(t *testing.T) {
	a, _ := NewSpec("sp-1", "orig", "slot-1")
	b, _ := NewSpec("sp-2", "renamed", "slot-1") // different id+name, identical runtime

	// Targets differ.
	b.SetTargets([]string{"agent-a", "agent-b"})

	if !a.RuntimeEquals(b) {
		t.Error("specs with identical runtime fields but different metadata should compare equal")
	}
}

// Every field backing SpecToProto must flip RuntimeEquals to false.
// Covers slot, kindType, kindConfig, timeoutMs, restartType, intervalMs,
// backoff, runnerLabels.
func TestRuntimeEqualsDetectsEveryRuntimeField(t *testing.T) {
	base := func() *Spec {
		s, _ := NewSpec("sp-1", "demo", "slot-1")
		s.SetKindConfig(map[string]any{"command": "sleep", "args": []any{"1"}})
		s.SetRunnerLabels(map[string]string{"zone": "eu"})
		return s
	}

	cases := []struct {
		name   string
		mutate func(*Spec)
	}{
		{"kindType", func(s *Spec) { s.SetKindType(enum.TaskKindWasm) }},
		{"kindConfig value", func(s *Spec) { s.SetKindConfig(map[string]any{"command": "echo"}) }},
		{"timeoutMs", func(s *Spec) { s.SetTimeoutMs(60_000) }},
		{"restartType", func(s *Spec) { s.SetRestartType(enum.RestartAlways) }},
		{"intervalMs", func(s *Spec) { s.SetIntervalMs(500) }},
		{"backoff", func(s *Spec) {
			s.SetBackoff(BackoffConfig{Jitter: enum.JitterFull, FirstMs: 2000, MaxMs: 10_000, Factor: 3.0})
		}},
		{"runnerLabels", func(s *Spec) { s.SetRunnerLabels(map[string]string{"zone": "us"}) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := base()
			b := base()
			if !a.RuntimeEquals(b) {
				t.Fatal("precondition: base specs must be RuntimeEqual")
			}
			tc.mutate(b)
			if a.RuntimeEquals(b) {
				t.Errorf("%s change not detected by RuntimeEquals", tc.name)
			}
		})
	}
}

// slot is immutable (no setter), so it can't be covered by the mutator
// table above — build two specs with different slots directly. RuntimeEquals
// must still treat slot as a runtime field.
func TestRuntimeEqualsDetectsSlot(t *testing.T) {
	a, _ := NewSpec("sp-1", "demo", "slot-1")
	b, _ := NewSpec("sp-1", "demo", "slot-2")
	if a.RuntimeEquals(b) {
		t.Error("slot difference not detected by RuntimeEquals")
	}
}

// Nil handling: both nil → equal; one nil → not equal. Keeps the
// caller-site ergonomics simple ("if !new.RuntimeEquals(old) …") even
// when old is missing on create.
func TestRuntimeEqualsHandlesNil(t *testing.T) {
	a, _ := NewSpec("sp-1", "demo", "slot-1")

	var n *Spec
	if a.RuntimeEquals(n) {
		t.Error("non-nil vs nil should not be equal")
	}
	if n.RuntimeEquals(a) {
		t.Error("nil vs non-nil should not be equal")
	}
	if !n.RuntimeEquals(n) {
		t.Error("nil vs nil should be equal")
	}
}
