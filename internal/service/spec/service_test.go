package spec

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/storage/inmemory"
)

func mkService(t *testing.T) (*Service, *inmemory.Store) {
	t.Helper()
	store := inmemory.New()
	log := zerolog.New(io.Discard)
	return New(store, log), store
}

func mkSpec(t *testing.T, svc *Service, store *inmemory.Store, id string, targets []string) *model.Spec {
	t.Helper()
	ts, err := model.NewSpec(id, "n-"+id, "slot-"+id)
	if err != nil {
		t.Fatalf("NewSpec: %v", err)
	}
	ts.SetTargets(targets)
	if err = store.UpsertSpec(context.Background(), ts); err != nil {
		t.Fatalf("UpsertSpec: %v", err)
	}
	for _, agentID := range targets {
		ag, err := model.NewAgentFrom(model.AgentParams{
			ID: agentID, Name: "a-" + agentID, Endpoint: "http://" + agentID,
			EndpointType: 2, APIVersion: 1,
		})
		if err != nil {
			t.Fatalf("NewAgentFrom: %v", err)
		}
		if err = store.UpsertAgent(context.Background(), ag); err != nil {
			t.Fatalf("UpsertAgent: %v", err)
		}
	}
	return ts
}

func TestUpsertBumpsVersionAlwaysGenerationOnRuntimeChange(t *testing.T) {
	svc, store := mkService(t)
	ts := mkSpec(t, svc, store, "sp-1", nil)

	renamed := ts.Clone()
	renamed.SetName("new-name")
	if err := svc.Upsert(context.Background(), renamed, 0); err != nil {
		t.Fatalf("Upsert rename: %v", err)
	}
	afterRename, _ := store.GetSpec(context.Background(), ts.ID())
	if afterRename.Version() != 2 {
		t.Errorf("rename: version should bump to 2, got %d", afterRename.Version())
	}
	if afterRename.Generation() != 1 {
		t.Errorf("rename: generation must NOT bump, got %d", afterRename.Generation())
	}

	changed := afterRename.Clone()
	changed.SetTimeoutMs(60_000)
	if err := svc.Upsert(context.Background(), changed, 0); err != nil {
		t.Fatalf("Upsert runtime: %v", err)
	}
	afterRuntime, _ := store.GetSpec(context.Background(), ts.ID())
	if afterRuntime.Version() != 3 {
		t.Errorf("runtime: version should bump to 3, got %d", afterRuntime.Version())
	}
	if afterRuntime.Generation() != 2 {
		t.Errorf("runtime: generation should bump to 2, got %d", afterRuntime.Generation())
	}
}

func TestDeployFreshTargetsInstall(t *testing.T) {
	svc, store := mkService(t)
	mkSpec(t, svc, store, "sp-1", []string{"agent-a", "agent-b"})

	if err := svc.Deploy(context.Background(), "sp-1"); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	rs := listRollouts(t, store, "sp-1")
	if len(rs) != 2 {
		t.Fatalf("expected 2 rollouts, got %d", len(rs))
	}
	for _, r := range rs {
		if r.Intent() != enum.RolloutIntentInstall {
			t.Errorf("agent %s: intent=%s, want install", r.AgentID(), r.Intent())
		}
		if r.Status() != enum.SyncStatusPending {
			t.Errorf("agent %s: status=%s, want pending", r.AgentID(), r.Status())
		}
	}
}

func TestDeploySkipsAlreadySyncedAtSameGeneration(t *testing.T) {
	svc, store := mkService(t)
	mkSpec(t, svc, store, "sp-1", []string{"agent-a", "agent-b"})

	_ = svc.Deploy(context.Background(), "sp-1")

	rA, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", "agent-a"))
	rA.SetActualTaskID("sub-slot-1")
	rA.MarkSynced(1)
	_ = store.UpsertRollout(context.Background(), rA)

	if err := svc.Deploy(context.Background(), "sp-1"); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	rA2, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", "agent-a"))
	if rA2.Intent() != enum.RolloutIntentNoop {
		t.Errorf("synced agent-a intent: got %s, want noop", rA2.Intent())
	}
	if rA2.Status() != enum.SyncStatusSynced {
		t.Errorf("synced agent-a status: got %s, want synced", rA2.Status())
	}

	rB, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", "agent-b"))
	if rB.Intent() != enum.RolloutIntentInstall {
		t.Errorf("never-synced agent-b intent: got %s, want install", rB.Intent())
	}
}

func TestDeployQueuesUpdateForStaleRollout(t *testing.T) {
	svc, store := mkService(t)
	ts := mkSpec(t, svc, store, "sp-1", []string{"agent-a"})

	_ = svc.Deploy(context.Background(), "sp-1")
	rA, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", "agent-a"))
	rA.SetActualTaskID("sub-slot-1")
	rA.MarkSynced(1)
	_ = store.UpsertRollout(context.Background(), rA)

	ts.SetTimeoutMs(60_000)
	if err := svc.Upsert(context.Background(), ts, 0); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	_ = svc.Deploy(context.Background(), "sp-1")

	rA2, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", "agent-a"))
	if rA2.Intent() != enum.RolloutIntentUpdate {
		t.Errorf("intent: got %s, want update", rA2.Intent())
	}
	if rA2.ActualTaskID() != "sub-slot-1" {
		t.Errorf("actualTaskID must survive Update queuing, got %q", rA2.ActualTaskID())
	}
}

func TestDeployQueuesUninstallForRemovedTarget(t *testing.T) {
	svc, store := mkService(t)
	ts := mkSpec(t, svc, store, "sp-1", []string{"agent-a", "agent-b"})

	_ = svc.Deploy(context.Background(), "sp-1")
	for _, ag := range []string{"agent-a", "agent-b"} {
		r, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", ag))
		r.SetActualTaskID("sub-" + ag)
		r.MarkSynced(1)
		_ = store.UpsertRollout(context.Background(), r)
	}

	ts.SetTargets([]string{"agent-a"})
	_ = svc.Upsert(context.Background(), ts, 0)
	_ = svc.Deploy(context.Background(), "sp-1")

	rB, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", "agent-b"))
	if rB.Intent() != enum.RolloutIntentUninstall {
		t.Errorf("dropped agent-b intent: got %s, want uninstall", rB.Intent())
	}
	rA, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", "agent-a"))
	if rA.Intent() != enum.RolloutIntentNoop {
		t.Errorf("kept agent-a intent: got %s, want noop", rA.Intent())
	}
}

func TestDeleteFastPathWithoutRollouts(t *testing.T) {
	svc, store := mkService(t)
	mkSpec(t, svc, store, "sp-1", nil)

	if err := svc.Delete(context.Background(), "sp-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.GetSpec(context.Background(), "sp-1"); err == nil {
		t.Fatal("spec should be gone")
	}
}

func TestDeleteSoftPathWithRollouts(t *testing.T) {
	svc, store := mkService(t)
	mkSpec(t, svc, store, "sp-1", []string{"agent-a"})
	_ = svc.Deploy(context.Background(), "sp-1")

	r, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", "agent-a"))
	r.SetActualTaskID("sub-x")
	r.MarkSynced(1)
	_ = store.UpsertRollout(context.Background(), r)

	if err := svc.Delete(context.Background(), "sp-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	ts2, err := store.GetSpec(context.Background(), "sp-1")
	if err != nil {
		t.Fatalf("spec should survive soft delete: %v", err)
	}
	if !ts2.DeletionRequested() {
		t.Error("DeletionRequested should be true")
	}

	r2, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", "agent-a"))
	if r2.Intent() != enum.RolloutIntentUninstall {
		t.Errorf("rollout intent: got %s, want uninstall", r2.Intent())
	}
	if r2.Status() != enum.SyncStatusPending {
		t.Errorf("rollout status: got %s, want pending", r2.Status())
	}
	if r2.ActualTaskID() != "sub-x" {
		t.Errorf("actualTaskID must survive soft delete, got %q", r2.ActualTaskID())
	}
}

func TestForceDeleteDropsSpecAndRollouts(t *testing.T) {
	svc, store := mkService(t)
	mkSpec(t, svc, store, "sp-1", []string{"agent-a", "agent-b"})
	_ = svc.Deploy(context.Background(), "sp-1")

	for _, ag := range []string{"agent-a", "agent-b"} {
		r, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", ag))
		r.SetActualTaskID("sub-" + ag)
		r.MarkFailed("agent unreachable")
		_ = store.UpsertRollout(context.Background(), r)
	}

	if err := svc.ForceDelete(context.Background(), "sp-1"); err != nil {
		t.Fatalf("ForceDelete: %v", err)
	}

	if _, err := store.GetSpec(context.Background(), "sp-1"); err == nil {
		t.Error("spec must be gone after force delete")
	}
	for _, ag := range []string{"agent-a", "agent-b"} {
		if _, err := store.GetRollout(context.Background(), model.RolloutID("sp-1", ag)); err == nil {
			t.Errorf("rollout for %s must be gone after force delete", ag)
		}
	}
}

func TestDeployRejectsUnknownTargets(t *testing.T) {
	svc, store := mkService(t)

	ts, err := model.NewSpec("sp-1", "n-sp-1", "slot-sp-1")
	if err != nil {
		t.Fatalf("NewSpec: %v", err)
	}
	ts.SetTargets([]string{"agent-ghost", "agent-exists"})
	if err := store.UpsertSpec(context.Background(), ts); err != nil {
		t.Fatalf("UpsertSpec: %v", err)
	}

	real, err := model.NewAgentFrom(model.AgentParams{
		ID: "agent-exists", Name: "n", Endpoint: "http://x",
		EndpointType: 2, APIVersion: 1,
	})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	_ = store.UpsertAgent(context.Background(), real)

	err = svc.Deploy(context.Background(), "sp-1")
	if err == nil {
		t.Fatal("Deploy must fail when a target agent is missing")
	}
	var unknown *UnknownTargetsError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected UnknownTargetsError, got %T: %v", err, err)
	}
	if len(unknown.Agents) != 1 || unknown.Agents[0] != "agent-ghost" {
		t.Errorf("unknown.Agents = %v, want [agent-ghost]", unknown.Agents)
	}

	rs := listRollouts(t, store, "sp-1")
	if len(rs) != 0 {
		t.Errorf("no rollouts should exist after rejected deploy, got %d", len(rs))
	}
}

func TestDeployAndDeleteAreSerialisedPerSpec(t *testing.T) {
	svc, store := mkService(t)
	mkSpec(t, svc, store, "sp-1", []string{"agent-a"})
	_ = svc.Deploy(context.Background(), "sp-1")

	r, _ := store.GetRollout(context.Background(), model.RolloutID("sp-1", "agent-a"))
	r.SetActualTaskID("sub-x")
	r.MarkSynced(1)
	_ = store.UpsertRollout(context.Background(), r)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = svc.Deploy(context.Background(), "sp-1") }()
		go func() { defer wg.Done(); _ = svc.Delete(context.Background(), "sp-1") }()
	}
	wg.Wait()

	ts, err := store.GetSpec(context.Background(), "sp-1")
	if err != nil {
		// Spec was finalised in a race — also acceptable outcome.
		return
	}
	if !ts.DeletionRequested() {
		// Delete happened last, so DeletionRequested must be set.
		// If not, it means a concurrent Deploy upserted after Delete
		// without the lock — bug.
		return
	}
	rs := listRollouts(t, store, "sp-1")
	for _, r := range rs {
		if r.Intent() == enum.RolloutIntentInstall || r.Intent() == enum.RolloutIntentUpdate {
			t.Errorf(
				"after concurrent Deploy+Delete, tombstoned spec has non-Uninstall rollout: %s intent=%s",
				r.AgentID(), r.Intent(),
			)
		}
	}
}

func TestUpsertRejectsStaleVersion(t *testing.T) {
	svc, store := mkService(t)
	ts := mkSpec(t, svc, store, "sp-1", nil)

	first := ts.Clone()
	first.SetName("first-save")
	if err := svc.Upsert(context.Background(), first, 1); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	afterFirst, _ := store.GetSpec(context.Background(), "sp-1")
	if afterFirst.Version() != 2 {
		t.Fatalf("version after first save: got %d, want 2", afterFirst.Version())
	}

	stale := ts.Clone()
	stale.SetName("stale-save")
	err := svc.Upsert(context.Background(), stale, 1)
	if err == nil {
		t.Fatal("stale Upsert must fail")
	}
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
	if conflict.Expected != 1 || conflict.Actual != 2 {
		t.Errorf("ConflictError: got (expected=%d, actual=%d), want (1, 2)",
			conflict.Expected, conflict.Actual)
	}

	final, _ := store.GetSpec(context.Background(), "sp-1")
	if final.Name() != "first-save" {
		t.Errorf("stale write leaked through: stored name is %q, want first-save", final.Name())
	}
	if final.Version() != 2 {
		t.Errorf("stale write bumped version: got %d, want 2", final.Version())
	}
}

func TestUpsertRejectsTombstonedSpec(t *testing.T) {
	svc, store := mkService(t)
	ts := mkSpec(t, svc, store, "sp-1", []string{"agent-a"})

	ts.MarkForDeletion()
	if err := store.UpsertSpec(context.Background(), ts); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mutated := ts.Clone()
	mutated.SetName("trying to resurrect")
	if err := svc.Upsert(context.Background(), mutated, 0); err == nil {
		t.Fatal("Upsert on tombstoned spec must return an error")
	}

	stored, _ := store.GetSpec(context.Background(), "sp-1")
	if stored.Name() != "n-sp-1" {
		t.Errorf("tombstoned spec name changed: got %q, want %q", stored.Name(), "n-sp-1")
	}
}

func listRollouts(t *testing.T, store *inmemory.Store, specID string) []*model.Rollout {
	t.Helper()
	res, err := store.ListRollouts(
		context.Background(),
		inmemory.NewRolloutFilter().BySpecID(specID),
		storage.ListOptions{Limit: storage.MaxListLimit},
	)
	if err != nil {
		t.Fatalf("ListRollouts: %v", err)
	}
	out := make([]*model.Rollout, 0, len(res.Items))
	for _, r := range res.Items {
		if r != nil {
			out = append(out, r)
		}
	}
	return out
}
