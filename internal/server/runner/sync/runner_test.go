package sync

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/rs/zerolog"

	genv1 "github.com/soltiHQ/control-plane/api/gen/v1"
	proxyv1 "github.com/soltiHQ/control-plane/api/proxy/v1"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/proxy"
	"github.com/soltiHQ/control-plane/internal/storage/inmemory"
)

// --- fakes ---

// fakeProxy records every call. Tests configure return values up front
// and assert on the recorded call sequence afterwards.
type fakeProxy struct {
	submits      []*genv1.CreateSpec
	submitResp   []string // pop from front, one per Submit call
	submitErr    []error
	deletes      []string
	deleteErr    []error
	gets         []string
	listTaskRuns []string
	listCalls    int
}

func (f *fakeProxy) ListTasks(ctx context.Context, _ proxy.TaskFilter) (*proxyv1.TaskListResponse, error) {
	f.listCalls++
	return &proxyv1.TaskListResponse{}, nil
}

func (f *fakeProxy) SubmitTask(ctx context.Context, sub proxy.TaskSubmission) (string, error) {
	f.submits = append(f.submits, sub.Spec)
	var id string
	var err error
	if len(f.submitResp) > 0 {
		id = f.submitResp[0]
		f.submitResp = f.submitResp[1:]
	}
	if len(f.submitErr) > 0 {
		err = f.submitErr[0]
		f.submitErr = f.submitErr[1:]
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

func (f *fakeProxy) GetTask(ctx context.Context, id string) (*proxyv1.TaskStatusResponse, error) {
	f.gets = append(f.gets, id)
	return &proxyv1.TaskStatusResponse{}, nil
}

func (f *fakeProxy) ListTaskRuns(ctx context.Context, id string) (*proxyv1.TaskRunListResponse, error) {
	f.listTaskRuns = append(f.listTaskRuns, id)
	return &proxyv1.TaskRunListResponse{}, nil
}

func (f *fakeProxy) StreamTaskLogs(ctx context.Context, _ string) (<-chan *genv1.OutputEventProto, error) {
	ch := make(chan *genv1.OutputEventProto)
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
		stop:   make(chan struct{}),
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

// Install: SubmitTask once, save TaskId, move to Synced.
func TestReconcileInstallSavesTaskIDAndSyncs(t *testing.T) {
	fp := &fakeProxy{submitResp: []string{"sub-slot-1"}}
	r, store, _ := newFakeRunner(t, fp)
	_ = seedSpecAndAgent(t, store, "sp-1", "agent-a")

	ro, _ := model.NewRollout("sp-1", "agent-a", 1)
	ro.SetIntent(enum.RolloutIntentInstall)
	_ = store.UpsertRollout(context.Background(), ro)

	r.reconcile(context.Background(), ro.ID())

	if len(fp.submits) != 1 {
		t.Fatalf("expected 1 SubmitTask call, got %d", len(fp.submits))
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

// Update: DeleteTask(old), SubmitTask(new), save new TaskId. Order matters.
func TestReconcileUpdateReCreatesAndSwapsTaskID(t *testing.T) {
	fp := &fakeProxy{submitResp: []string{"sub-slot-2"}}
	r, store, _ := newFakeRunner(t, fp)
	_ = seedSpecAndAgent(t, store, "sp-1", "agent-a")

	ro, _ := model.NewRollout("sp-1", "agent-a", 1)
	ro.SetActualTaskID("sub-slot-old")
	ro.MarkSynced(1)
	ro.SetIntent(enum.RolloutIntentUpdate)
	ro.MarkPending(2)
	_ = store.UpsertRollout(context.Background(), ro)

	r.reconcile(context.Background(), ro.ID())

	if len(fp.deletes) != 1 || fp.deletes[0] != "sub-slot-old" {
		t.Errorf("expected DeleteTask(sub-slot-old), got %v", fp.deletes)
	}
	if len(fp.submits) != 1 {
		t.Errorf("expected 1 SubmitTask, got %d", len(fp.submits))
	}

	after, _ := store.GetRollout(context.Background(), ro.ID())
	if after.ActualTaskID() != "sub-slot-2" {
		t.Errorf("actualTaskID: got %q, want sub-slot-2", after.ActualTaskID())
	}
	if after.Status() != enum.SyncStatusSynced {
		t.Errorf("status: got %s, want synced", after.Status())
	}
}

// Update: DeleteTask succeeds but SubmitTask fails → ActualTaskID must
// have been cleared before SubmitTask so the next tick resumes cleanly.
func TestReconcileUpdateClearsActualTaskIDBeforeSubmitOnSubmitFailure(t *testing.T) {
	submitErr := errors.New("agent down")
	fp := &fakeProxy{
		submitResp: []string{""},
		submitErr:  []error{submitErr},
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

	after, _ := store.GetRollout(context.Background(), ro.ID())
	if after.ActualTaskID() != "" {
		t.Errorf("ActualTaskID must be cleared after successful DeleteTask (got %q)", after.ActualTaskID())
	}
	if after.Status() != enum.SyncStatusFailed {
		t.Errorf("status: got %s, want failed", after.Status())
	}
	// Intent stays so the next tick retries.
	if after.Intent() != enum.RolloutIntentUpdate {
		t.Errorf("intent: got %s, want update", after.Intent())
	}
}

// Update where DeleteTask returns NotFound → treat as success, proceed
// to SubmitTask. Agents reboot and lose state; CP shouldn't wedge.
func TestReconcileUpdateTreatsDeleteTaskNotFoundAsSuccess(t *testing.T) {
	fp := &fakeProxy{
		submitResp: []string{"sub-slot-new"},
		deleteErr:  []error{errors.New("proxy: unexpected status: 404 TaskNotFound: not found")},
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

	after, _ := store.GetRollout(context.Background(), ro.ID())
	if after.Status() != enum.SyncStatusSynced {
		t.Errorf("status: got %s, want synced (NotFound should be treated as success)", after.Status())
	}
	if after.ActualTaskID() != "sub-slot-new" {
		t.Errorf("actualTaskID: got %q, want sub-slot-new", after.ActualTaskID())
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
	if len(fp.submits) != 0 {
		t.Errorf("uninstall must not submit; got %d", len(fp.submits))
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
