package sync

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/rs/zerolog"

	taskv1 "github.com/soltiHQ/control-plane/api/gen/solti/task/v1"
	proxyv1 "github.com/soltiHQ/control-plane/api/proxy/v1"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/proxy"
	"github.com/soltiHQ/control-plane/internal/storage/inmemory"
	"github.com/soltiHQ/control-plane/internal/transport/errkind"
)

// agentNotFound mirrors what the proxy returns for an agent-side 404: an error
// that errkind classifies as NotFound via its ErrorKind() escape hatch. Tests
// feed this (not a bare string) so isNotFound matches production behavior.
type agentNotFound struct{}

func (agentNotFound) Error() string           { return "proxy: task not found" }
func (agentNotFound) ErrorKind() errkind.Kind { return errkind.NotFound }

// --- fakes ---

// fakeProxy records every call. Tests configure return values up front
// and assert on the recorded call sequence afterwards.
type fakeProxy struct {
	applies      []*taskv1.CreateSpec
	applyResp    []string // pop from front, one per Apply call
	applyErr     []error
	deletes      []string
	deleteErr    []error
	gets         []string
	listTaskRuns []string
	listCalls    int
}

func (f *fakeProxy) ListTasks(ctx context.Context, _ proxy.TaskFilter) (*proxyv1.ListTasksResponse, error) {
	f.listCalls++
	return &proxyv1.ListTasksResponse{}, nil
}

func (f *fakeProxy) ApplyTask(ctx context.Context, sub proxy.TaskSubmission) (string, error) {
	f.applies = append(f.applies, sub.Spec)
	var id string
	var err error
	if len(f.applyResp) > 0 {
		id = f.applyResp[0]
		f.applyResp = f.applyResp[1:]
	}
	if len(f.applyErr) > 0 {
		err = f.applyErr[0]
		f.applyErr = f.applyErr[1:]
	}
	return id, err
}

func (f *fakeProxy) DeleteTask(ctx context.Context, id string) error {
	f.deletes = append(f.deletes, id)
	if len(f.deleteErr) > 0 {
		err := f.deleteErr[0]
		f.deleteErr = f.deleteErr[1:]
		return err
	}
	return nil
}

func (f *fakeProxy) GetTask(ctx context.Context, id string) (*proxyv1.GetTaskResponse, error) {
	f.gets = append(f.gets, id)
	return &proxyv1.GetTaskResponse{}, nil
}

func (f *fakeProxy) ListTaskRuns(ctx context.Context, id string) (*proxyv1.ListTaskRunsResponse, error) {
	f.listTaskRuns = append(f.listTaskRuns, id)
	return &proxyv1.ListTaskRunsResponse{}, nil
}

func (f *fakeProxy) StreamTaskLogs(ctx context.Context, _ string) (<-chan *taskv1.StreamTaskLogsResponse, error) {
	ch := make(chan *taskv1.StreamTaskLogsResponse)
	close(ch)
	return ch, nil
}

type fakePool struct{ ap *fakeProxy }

func (p *fakePool) Get(_ string, _ enum.EndpointType, _ enum.APIVersion) (proxy.AgentProxy, error) {
	return p.ap, nil
}

// --- test helpers ---

func newFakeRunner(t *testing.T, fp *fakeProxy) (*Runner, *inmemory.Store, *event.Hub) {
	t.Helper()
	store := inmemory.New()
	hub := event.NewHub(zerolog.New(io.Discard))
	r := &Runner{
		pool:   &fakePool{ap: fp},
		hub:    hub,
		logger: zerolog.New(io.Discard),
		store:  store,
		cfg:    Config{}.withDefaults(),
	}
	return r, store, hub
}

func seedSpecAndAgent(t *testing.T, store *inmemory.Store, specID, agentID string) *model.Spec {
	t.Helper()
	ts, err := model.NewSpec(specID, "n", "slot-"+specID)
	if err != nil {
		t.Fatalf("NewSpec: %v", err)
	}
	ts.SetTargets([]string{agentID})
	ts.SetKindConfig(map[string]any{"command": map[string]any{"command": "echo", "args": []any{"hi"}}})
	if err := store.UpsertSpec(context.Background(), ts); err != nil {
		t.Fatalf("UpsertSpec: %v", err)
	}
	ag, err := model.NewAgentFrom(model.AgentParams{
		ID:           agentID,
		Name:         "a-" + agentID,
		Endpoint:     "http://agent",
		EndpointType: 2, // proto enum for HTTP
		APIVersion:   1,
	})
	if err != nil {
		t.Fatalf("NewAgentFrom: %v", err)
	}
	if err := store.UpsertAgent(context.Background(), ag); err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	return ts
}

// --- tests ---

// Install: ApplyTask once, save TaskId, move to Synced.
func TestReconcileInstallSavesTaskIDAndSyncs(t *testing.T) {
	fp := &fakeProxy{applyResp: []string{"sub-slot-1"}}
	r, store, _ := newFakeRunner(t, fp)
	_ = seedSpecAndAgent(t, store, "sp-1", "agent-a")

	ro, _ := model.NewRollout("sp-1", "agent-a", 1)
	ro.SetIntent(enum.RolloutIntentInstall)
	_ = store.UpsertRollout(context.Background(), ro)

	r.reconcile(context.Background(), ro.ID())

	if len(fp.applies) != 1 {
		t.Fatalf("expected 1 ApplyTask call, got %d", len(fp.applies))
	}
	if len(fp.deletes) != 0 {
		t.Errorf("install must not delete; got %d", len(fp.deletes))
	}

	after, _ := store.GetRollout(context.Background(), ro.ID())
	if after.ActualTaskID() != "sub-slot-1" {
		t.Errorf("actualTaskID: got %q, want sub-slot-1", after.ActualTaskID())
	}
	if after.Status() != enum.SyncStatusSynced {
		t.Errorf("status: got %s, want synced", after.Status())
	}
	if after.Intent() != enum.RolloutIntentNoop {
		t.Errorf("intent: got %s, want noop after synced", after.Intent())
	}
}

// Update: ApplyTask supersedes in a single call (no DeleteTask), save new TaskId.
func TestReconcileUpdateAppliesAndSwapsTaskID(t *testing.T) {
	fp := &fakeProxy{applyResp: []string{"sub-slot-2"}}
	r, store, _ := newFakeRunner(t, fp)
	_ = seedSpecAndAgent(t, store, "sp-1", "agent-a")

	ro, _ := model.NewRollout("sp-1", "agent-a", 1)
	ro.SetActualTaskID("sub-slot-old")
	ro.MarkSynced(1)
	ro.SetIntent(enum.RolloutIntentUpdate)
	ro.MarkPending(2)
	_ = store.UpsertRollout(context.Background(), ro)

	r.reconcile(context.Background(), ro.ID())

	if len(fp.deletes) != 0 {
		t.Errorf("update must not delete (ApplyTask supersedes); got %v", fp.deletes)
	}
	if len(fp.applies) != 1 {
		t.Errorf("expected 1 ApplyTask, got %d", len(fp.applies))
	}

	after, _ := store.GetRollout(context.Background(), ro.ID())
	if after.ActualTaskID() != "sub-slot-2" {
		t.Errorf("actualTaskID: got %q, want sub-slot-2", after.ActualTaskID())
	}
	if after.Status() != enum.SyncStatusSynced {
		t.Errorf("status: got %s, want synced", after.Status())
	}
}

// Update where ApplyTask fails → mark Failed, keep the old ActualTaskID
// (ApplyTask is atomic: a failed apply leaves the running task untouched),
// and keep Intent=Update so the next tick retries.
func TestReconcileUpdateApplyFailureKeepsOldTaskID(t *testing.T) {
	applyErr := errors.New("agent down")
	fp := &fakeProxy{
		applyResp: []string{""},
		applyErr:  []error{applyErr},
	}
	r, store, _ := newFakeRunner(t, fp)
	_ = seedSpecAndAgent(t, store, "sp-1", "agent-a")

	ro, _ := model.NewRollout("sp-1", "agent-a", 1)
	ro.SetActualTaskID("sub-slot-old")
	ro.MarkSynced(1)
	ro.SetIntent(enum.RolloutIntentUpdate)
	ro.MarkPending(2)
	_ = store.UpsertRollout(context.Background(), ro)

	r.reconcile(context.Background(), ro.ID())

	if len(fp.deletes) != 0 {
		t.Errorf("apply path must not delete; got %v", fp.deletes)
	}
	after, _ := store.GetRollout(context.Background(), ro.ID())
	if after.ActualTaskID() != "sub-slot-old" {
		t.Errorf("ActualTaskID must be untouched on apply failure (got %q)", after.ActualTaskID())
	}
	if after.Status() != enum.SyncStatusFailed {
		t.Errorf("status: got %s, want failed", after.Status())
	}
	// Intent stays so the next tick retries.
	if after.Intent() != enum.RolloutIntentUpdate {
		t.Errorf("intent: got %s, want update", after.Intent())
	}
}

// Uninstall where DeleteTask returns NotFound → treat as success, drop the
// rollout. Agents reboot and lose state; CP shouldn't wedge.
func TestReconcileUninstallTreatsDeleteTaskNotFoundAsSuccess(t *testing.T) {
	fp := &fakeProxy{
		deleteErr: []error{agentNotFound{}},
	}
	r, store, _ := newFakeRunner(t, fp)
	_ = seedSpecAndAgent(t, store, "sp-1", "agent-a")

	ro, _ := model.NewRollout("sp-1", "agent-a", 1)
	ro.SetActualTaskID("sub-slot-old")
	ro.MarkSynced(1)
	ro.SetIntent(enum.RolloutIntentUninstall)
	ro.MarkPending(2)
	_ = store.UpsertRollout(context.Background(), ro)

	r.reconcile(context.Background(), ro.ID())

	if len(fp.deletes) != 1 {
		t.Errorf("expected 1 DeleteTask, got %v", fp.deletes)
	}
	if _, err := store.GetRollout(context.Background(), ro.ID()); err == nil {
		t.Error("rollout row should be dropped even when DeleteTask returns NotFound")
	}
}

// Uninstall with an ActualTaskID: DeleteTask, drop rollout row.
func TestReconcileUninstallDeletesTaskAndRolloutRow(t *testing.T) {
	fp := &fakeProxy{}
	r, store, _ := newFakeRunner(t, fp)
	_ = seedSpecAndAgent(t, store, "sp-1", "agent-a")

	ro, _ := model.NewRollout("sp-1", "agent-a", 1)
	ro.SetActualTaskID("sub-slot-bye")
	ro.MarkSynced(1)
	ro.SetIntent(enum.RolloutIntentUninstall)
	ro.MarkPending(1)
	_ = store.UpsertRollout(context.Background(), ro)

	r.reconcile(context.Background(), ro.ID())

	if len(fp.deletes) != 1 || fp.deletes[0] != "sub-slot-bye" {
		t.Errorf("expected DeleteTask(sub-slot-bye), got %v", fp.deletes)
	}
	if len(fp.applies) != 0 {
		t.Errorf("uninstall must not apply; got %d", len(fp.applies))
	}
	if _, err := store.GetRollout(context.Background(), ro.ID()); err == nil {
		t.Error("rollout row should be gone after uninstall")
	}
}

// Uninstall with empty ActualTaskID: no network call, just drop the row.
func TestReconcileUninstallSkipsNetworkForEmptyTaskID(t *testing.T) {
	fp := &fakeProxy{}
	r, store, _ := newFakeRunner(t, fp)
	_ = seedSpecAndAgent(t, store, "sp-1", "agent-a")

	ro, _ := model.NewRollout("sp-1", "agent-a", 1)
	ro.SetIntent(enum.RolloutIntentUninstall)
	ro.MarkPending(1)
	_ = store.UpsertRollout(context.Background(), ro)

	r.reconcile(context.Background(), ro.ID())

	if len(fp.deletes) != 0 {
		t.Errorf("no task id → no DeleteTask; got %v", fp.deletes)
	}
	if _, err := store.GetRollout(context.Background(), ro.ID()); err == nil {
		t.Error("rollout row should still be deleted")
	}
}

// Finalizer drops a DeletionRequested spec once its rollouts drain.
func TestFinalizeDeletedSpecsDropsEmptyTombstone(t *testing.T) {
	r, store, _ := newFakeRunner(t, &fakeProxy{})
	ts := seedSpecAndAgent(t, store, "sp-1", "agent-a")
	ts.MarkForDeletion()
	_ = store.UpsertSpec(context.Background(), ts)

	// No rollouts for sp-1 — finalizer should drop the spec immediately.
	r.finalizeDeletedSpecs(context.Background())

	if _, err := store.GetSpec(context.Background(), "sp-1"); err == nil {
		t.Error("spec should have been finalized (deleted)")
	}
}

// Finalizer leaves DeletionRequested specs alone while rollouts remain.
// This protects the tombstone state from being removed before the sync
// runner can uninstall each agent.
func TestFinalizeDeletedSpecsKeepsTombstoneWithRollouts(t *testing.T) {
	r, store, _ := newFakeRunner(t, &fakeProxy{})
	ts := seedSpecAndAgent(t, store, "sp-1", "agent-a")
	ts.MarkForDeletion()
	_ = store.UpsertSpec(context.Background(), ts)

	// Seed a rollout to block finalization.
	ro, _ := model.NewRollout("sp-1", "agent-a", 1)
	_ = store.UpsertRollout(context.Background(), ro)

	r.finalizeDeletedSpecs(context.Background())

	if _, err := store.GetSpec(context.Background(), "sp-1"); err != nil {
		t.Error("spec should still exist while rollouts remain")
	}
}
