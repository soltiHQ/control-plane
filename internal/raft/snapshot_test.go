package raft_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	raftv1 "github.com/soltiHQ/control-plane/api/gen/solti/raft/v1"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/event"
	raftpkg "github.com/soltiHQ/control-plane/internal/raft"
	"github.com/soltiHQ/control-plane/internal/storage/inmemory"
)

// bufSink is a minimal hraft.SnapshotSink backed by a bytes.Buffer. It
// satisfies io.WriteCloser + ID() + Cancel() without touching disk.
type bufSink struct {
	buf       *bytes.Buffer
	cancelled bool
	closed    bool
}

func newBufSink() *bufSink                     { return &bufSink{buf: &bytes.Buffer{}} }
func (s *bufSink) Write(p []byte) (int, error) { return s.buf.Write(p) }
func (s *bufSink) Close() error                { s.closed = true; return nil }
func (s *bufSink) ID() string                  { return "test-snapshot" }
func (s *bufSink) Cancel() error               { s.cancelled = true; return nil }
func (s *bufSink) reader() io.ReadCloser       { return io.NopCloser(bytes.NewReader(s.buf.Bytes())) }

// populated holds the auto-generated IDs of entities populateStore created,
// for later GET lookups in the assertions.
type populated struct {
	rolloutID string
}

// populateStore inserts one of every entity type so a round-trip test
// exercises every encode/decode path.
func populateStore(t *testing.T, store *inmemory.Store) populated {
	t.Helper()
	ctx := context.Background()

	a, err := model.NewAgent("a1", "agent-1", "http://a")
	if err != nil {
		t.Fatal(err)
	}
	a.SetOS("linux")
	a.SetArch("amd64")
	a.LabelAdd("env", "prod")
	if err := store.UpsertAgent(ctx, a); err != nil {
		t.Fatal(err)
	}

	u, err := model.NewUser("u1", "subj-1")
	if err != nil {
		t.Fatal(err)
	}
	u.NameAdd("Alice")
	u.EmailAdd("a@example.com")
	if err := store.UpsertUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	r, err := model.NewRole("r1", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRole(ctx, r); err != nil {
		t.Fatal(err)
	}

	c, err := model.NewCredential("c1", "u1", enum.Password)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertCredential(ctx, c); err != nil {
		t.Fatal(err)
	}

	v, err := model.NewVerifier("v1", "c1", enum.Password)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertVerifier(ctx, v); err != nil {
		t.Fatal(err)
	}

	sess, err := model.NewSession("s1", "u1", "c1", enum.Password, []byte("hash"), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}

	sp, err := model.NewSpec("sp1", "spec-1", "slot-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSpec(ctx, sp); err != nil {
		t.Fatal(err)
	}

	ro, err := model.NewRollout("sp1", "a1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertRollout(ctx, ro); err != nil {
		t.Fatal(err)
	}
	return populated{rolloutID: ro.ID()}
}

// TestFSM_SnapshotRoundtrip — full state survives a Persist→Restore cycle.
func TestFSM_SnapshotRoundtrip(t *testing.T) {
	srcStore := inmemory.New()
	src := populateStore(t, srcStore)

	srcFSM := raftpkg.NewFSM(srcStore, event.NewHub(zerolog.Nop()))
	snap, err := srcFSM.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	defer snap.Release()

	sink := newBufSink()
	if err := snap.Persist(sink); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if sink.cancelled {
		t.Fatal("sink cancelled on success")
	}
	if !sink.closed {
		t.Fatal("sink not closed after Persist")
	}

	dstStore := inmemory.New()
	dstFSM := raftpkg.NewFSM(dstStore, event.NewHub(zerolog.Nop()))
	if err := dstFSM.Restore(sink.reader()); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	ctx := context.Background()
	if got, err := dstStore.GetAgent(ctx, "a1"); err != nil || got.ID() != "a1" || got.OS() != "linux" {
		t.Fatalf("agent: %+v err=%v", got, err)
	}
	if got, err := dstStore.GetUser(ctx, "u1"); err != nil || got.ID() != "u1" || got.Name() != "Alice" {
		t.Fatalf("user: %+v err=%v", got, err)
	}
	if got, err := dstStore.GetRole(ctx, "r1"); err != nil || got.Name() != "admin" {
		t.Fatalf("role: %+v err=%v", got, err)
	}
	if got, err := dstStore.GetCredential(ctx, "c1"); err != nil || got.UserID() != "u1" {
		t.Fatalf("credential: %+v err=%v", got, err)
	}
	if got, err := dstStore.GetVerifier(ctx, "v1"); err != nil || got.CredentialID() != "c1" {
		t.Fatalf("verifier: %+v err=%v", got, err)
	}
	if got, err := dstStore.GetSession(ctx, "s1"); err != nil || got.UserID() != "u1" {
		t.Fatalf("session: %+v err=%v", got, err)
	}
	if got, err := dstStore.GetSpec(ctx, "sp1"); err != nil || got.Name() != "spec-1" {
		t.Fatalf("spec: %+v err=%v", got, err)
	}
	if got, err := dstStore.GetRollout(ctx, src.rolloutID); err != nil || got.SpecID() != "sp1" {
		t.Fatalf("rollout: %+v err=%v", got, err)
	}
}

// TestFSM_RestoreOverwritesExisting — Restore drops pre-existing data in
// the destination.
func TestFSM_RestoreOverwritesExisting(t *testing.T) {
	srcStore := inmemory.New()
	populateStore(t, srcStore)
	srcFSM := raftpkg.NewFSM(srcStore, event.NewHub(zerolog.Nop()))
	snap, err := srcFSM.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Release()
	sink := newBufSink()
	if err := snap.Persist(sink); err != nil {
		t.Fatal(err)
	}

	dstStore := inmemory.New()
	ctx := context.Background()
	stale, _ := model.NewAgent("a999", "stale", "http://stale")
	if err := dstStore.UpsertAgent(ctx, stale); err != nil {
		t.Fatal(err)
	}

	dstFSM := raftpkg.NewFSM(dstStore, event.NewHub(zerolog.Nop()))
	if err := dstFSM.Restore(sink.reader()); err != nil {
		t.Fatal(err)
	}
	if _, err := dstStore.GetAgent(ctx, "a999"); err == nil {
		t.Fatal("stale agent survived Restore — should have been dropped")
	}
	if _, err := dstStore.GetAgent(ctx, "a1"); err != nil {
		t.Fatalf("a1 missing after Restore: %v", err)
	}
}

// TestFSM_RestoreEmpty — Persist of an empty store + Restore yields an
// empty destination.
func TestFSM_RestoreEmpty(t *testing.T) {
	srcFSM := raftpkg.NewFSM(inmemory.New(), event.NewHub(zerolog.Nop()))
	snap, err := srcFSM.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Release()
	sink := newBufSink()
	if err := snap.Persist(sink); err != nil {
		t.Fatal(err)
	}

	dstStore := inmemory.New()
	dstFSM := raftpkg.NewFSM(dstStore, event.NewHub(zerolog.Nop()))
	if err := dstFSM.Restore(sink.reader()); err != nil {
		t.Fatal(err)
	}
	got := dstStore.SnapshotForRaft()
	total := len(got.Agents) + len(got.Users) + len(got.Roles) + len(got.Credentials) +
		len(got.Verifiers) + len(got.Sessions) + len(got.Specs) + len(got.Rollouts)
	if total != 0 {
		t.Fatalf("expected empty store, got %d entries", total)
	}
}

// TestFSM_RestoreBadMagic — wrong magic in the header is rejected.
func TestFSM_RestoreBadMagic(t *testing.T) {
	junk := &raftv1.Snapshot{Header: &raftv1.SnapshotHeader{Magic: "NOT-SOLTI", Version: 1}}
	data, err := proto.Marshal(junk)
	if err != nil {
		t.Fatal(err)
	}

	dstFSM := raftpkg.NewFSM(inmemory.New(), event.NewHub(zerolog.Nop()))
	if err := dstFSM.Restore(io.NopCloser(bytes.NewReader(data))); err == nil {
		t.Fatal("Restore accepted bad magic")
	}
}

// TestFSM_RestoreCorrupt — random bytes don't even unmarshal; Restore
// surfaces the proto error without touching the destination store.
func TestFSM_RestoreCorrupt(t *testing.T) {
	dstStore := inmemory.New()
	preexisting, _ := model.NewAgent("pre", "preexisting", "http://pre")
	_ = dstStore.UpsertAgent(context.Background(), preexisting)

	dstFSM := raftpkg.NewFSM(dstStore, event.NewHub(zerolog.Nop()))
	if err := dstFSM.Restore(io.NopCloser(bytes.NewReader([]byte("not a proto")))); err == nil {
		t.Fatal("Restore accepted garbage bytes")
	}
	// Atomic restore: dst should still hold the preexisting entry.
	if _, err := dstStore.GetAgent(context.Background(), "pre"); err != nil {
		t.Fatalf("preexisting agent dropped on failed restore: %v", err)
	}
}
