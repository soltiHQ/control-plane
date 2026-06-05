package model

import (
	"reflect"
	"slices"
	"time"

	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/enum"
)

var _ domain.Entity[*Spec] = (*Spec)(nil)

// BackoffConfig holds backoff parameters for task restart delays.
type BackoffConfig struct {
	Jitter  enum.JitterStrategy
	Factor  float64
	FirstMs int64
	MaxMs   int64
}

// Spec represents a desired task specification managed by the control-plane.
//
// A Spec is the "desired state" in the reconciliation model: it defines what
// task should run on which agents. The sync runner compares it against what the
// agents actually have (via Rollout records) and converges them.
//
// # Versioning: two counters
//
//   - Version bumps on every Upsert, even pure-metadata edits (rename,
//     target-list change). It is the UI-facing "edits counter" — used for audit
//     and as the optimistic-concurrency token (which save the user is on).
//   - Generation bumps only when a runtime field changes (one that SpecToProto
//     reads: slot, kind, timeout, restart/interval, backoff, runnerLabels).
//     Rollouts track ObservedGeneration; a re-create on the agent fires only
//     when ObservedGeneration != Generation. This stops pure-metadata edits from
//     churning live tasks on every agent. See RuntimeEquals.
//
// # Soft delete
//
// DeletionRequested flips on delete: the record survives so the sync runner can
// honor rollout uninstalls (DeleteTask on each agent) before the finalizer drops
// the row — the k8s deletionTimestamp + finalizer pattern.
type Spec struct {
	// CP-owned metadata: identity, versioning, targeting, timestamps.
	id                string
	name              string
	version           int               // edits counter; bumps on every Upsert
	generation        int               // runtime-change counter; drives re-create
	deletionRequested bool              // soft-delete tombstone flag
	targets           []string          // explicit target agent IDs
	targetLabels      map[string]string // label selector for dynamic targeting
	createdAt         time.Time
	updatedAt         time.Time

	// Task definition: the fields that mirror the agent's CreateSpec and decide
	// what actually runs. (Admission is intentionally absent — the CP always
	// pins Replace on the wire; see internal/proxy/convert.go.)
	//
	// slot is immutable: set once in NewSpec, never changed (there is no
	// SetSlot). It is the task's identity/lane on the agent — a different slot is
	// a new deployment, not an edit.
	slot         string
	kindType     enum.TaskKindType
	restartType  enum.RestartType
	timeoutMs    int64
	intervalMs   int64 // only meaningful for RestartAlways
	backoff      BackoffConfig
	kindConfig   map[string]any // backend config; shape depends on kindType
	runnerLabels map[string]string
}

// NewSpec creates a new Spec domain entity with sensible defaults.
func NewSpec(id, name, slot string) (*Spec, error) {
	if id == "" {
		return nil, domain.ErrEmptyID
	}
	if slot == "" {
		return nil, domain.ErrFieldEmpty
	}
	now := time.Now()
	return &Spec{
		createdAt: now,
		updatedAt: now,

		id:         id,
		name:       name,
		slot:       slot,
		version:    1,
		generation: 1,

		targets:      nil,
		targetLabels: make(map[string]string),
		kindType:     enum.TaskKindSubprocess,
		restartType:  enum.RestartNever,
		timeoutMs:    30000,
		intervalMs:   0,
		backoff: BackoffConfig{
			Jitter:  enum.JitterNone,
			FirstMs: 1000,
			MaxMs:   5000,
			Factor:  2.0,
		},
		kindConfig:   make(map[string]any),
		runnerLabels: make(map[string]string),
	}, nil
}

func (ts *Spec) ID() string              { return ts.id }
func (ts *Spec) Name() string            { return ts.name }
func (ts *Spec) Slot() string            { return ts.slot }
func (ts *Spec) Version() int            { return ts.version }
func (ts *Spec) Generation() int         { return ts.generation }
func (ts *Spec) DeletionRequested() bool { return ts.deletionRequested }
func (ts *Spec) CreatedAt() time.Time    { return ts.createdAt }
func (ts *Spec) UpdatedAt() time.Time    { return ts.updatedAt }

// SetCreatedAt / SetUpdatedAt / SetVersion / SetGeneration - used by persistence adapters to restore the exact state on reconstruction.
func (ts *Spec) SetCreatedAt(t time.Time)      { ts.createdAt = t }
func (ts *Spec) SetUpdatedAt(t time.Time)      { ts.updatedAt = t }
func (ts *Spec) SetVersion(v int)              { ts.version = v }
func (ts *Spec) SetGeneration(g int)           { ts.generation = g }
func (ts *Spec) SetDeletionRequested(b bool)   { ts.deletionRequested = b }
func (ts *Spec) KindType() enum.TaskKindType   { return ts.kindType }
func (ts *Spec) TimeoutMs() int64              { return ts.timeoutMs }
func (ts *Spec) RestartType() enum.RestartType { return ts.restartType }
func (ts *Spec) IntervalMs() int64             { return ts.intervalMs }
func (ts *Spec) Backoff() BackoffConfig        { return ts.backoff }

// KindConfig returns a defensive copy of the kind configuration.
func (ts *Spec) KindConfig() map[string]any {
	out := make(map[string]any, len(ts.kindConfig))
	for k, v := range ts.kindConfig {
		out[k] = v
	}
	return out
}

// Targets return a copy of the target agent IDs.
func (ts *Spec) Targets() []string {
	out := make([]string, len(ts.targets))
	copy(out, ts.targets)
	return out
}

// TargetLabels returns a defensive copy of the target label selector.
func (ts *Spec) TargetLabels() map[string]string {
	out := make(map[string]string, len(ts.targetLabels))
	for k, v := range ts.targetLabels {
		out[k] = v
	}
	return out
}

// RunnerLabels returns a defensive copy of the runner labels.
func (ts *Spec) RunnerLabels() map[string]string {
	out := make(map[string]string, len(ts.runnerLabels))
	for k, v := range ts.runnerLabels {
		out[k] = v
	}
	return out
}

func (ts *Spec) SetName(name string) {
	if ts.name == name {
		return
	}
	ts.name = name
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetKindType(kt enum.TaskKindType) {
	if ts.kindType == kt {
		return
	}
	ts.kindType = kt
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetKindConfig(cfg map[string]any) {
	if anyMapEqual(ts.kindConfig, cfg) {
		return
	}
	cp := make(map[string]any, len(cfg))
	for k, v := range cfg {
		cp[k] = v
	}
	ts.kindConfig = cp
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetTimeoutMs(ms int64) {
	if ts.timeoutMs == ms {
		return
	}
	ts.timeoutMs = ms
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetRestartType(rt enum.RestartType) {
	if ts.restartType == rt {
		return
	}
	ts.restartType = rt
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetIntervalMs(ms int64) {
	if ts.intervalMs == ms {
		return
	}
	ts.intervalMs = ms
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetBackoff(b BackoffConfig) {
	if ts.backoff == b {
		return
	}
	ts.backoff = b
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetTargets(targets []string) {
	if slices.Equal(ts.targets, targets) {
		return
	}
	cp := make([]string, len(targets))
	copy(cp, targets)
	ts.targets = cp
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetTargetLabels(labels map[string]string) {
	if stringMapEqual(ts.targetLabels, labels) {
		return
	}
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	ts.targetLabels = cp
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetRunnerLabels(labels map[string]string) {
	if stringMapEqual(ts.runnerLabels, labels) {
		return
	}
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	ts.runnerLabels = cp
	ts.updatedAt = time.Now()
}

// IncrementVersion bumps the edits counter (version) and updates the
// timestamp. Callers should invoke this on every Upsert.
func (ts *Spec) IncrementVersion() {
	ts.version++
	ts.updatedAt = time.Now()
}

// BumpGeneration bumps the runtime-change counter. Callers should invoke
// this on Upsert **only if** a field that `SpecToProto` reads has
// changed (slot, kind, timeout, restart/interval, backoff, admission,
// runnerLabels). Pure metadata edits (name, targets) must NOT bump
// generation — otherwise every rename would force a re-create on every
// agent.
func (ts *Spec) BumpGeneration() {
	ts.generation++
	ts.updatedAt = time.Now()
}

// MarkForDeletion flips the soft-delete flag. The Spec record survives
// until every Rollout for it is uninstalled; the sync runner's finalizer
// pass then calls `DeleteSpec` for real.
func (ts *Spec) MarkForDeletion() {
	if ts.deletionRequested {
		return
	}
	ts.deletionRequested = true
	ts.updatedAt = time.Now()
}

// Clone creates a deep copy of the Spec.
func (ts *Spec) Clone() *Spec {
	kindConfig := make(map[string]any, len(ts.kindConfig))
	for k, v := range ts.kindConfig {
		kindConfig[k] = v
	}
	targets := make([]string, len(ts.targets))
	copy(targets, ts.targets)
	targetLabels := make(map[string]string, len(ts.targetLabels))
	for k, v := range ts.targetLabels {
		targetLabels[k] = v
	}
	runnerLabels := make(map[string]string, len(ts.runnerLabels))
	for k, v := range ts.runnerLabels {
		runnerLabels[k] = v
	}

	return &Spec{
		id:                ts.id,
		name:              ts.name,
		version:           ts.version,
		generation:        ts.generation,
		deletionRequested: ts.deletionRequested,
		targets:           targets,
		targetLabels:      targetLabels,
		createdAt:         ts.createdAt,
		updatedAt:         ts.updatedAt,

		slot:         ts.slot,
		kindType:     ts.kindType,
		kindConfig:   kindConfig,
		timeoutMs:    ts.timeoutMs,
		restartType:  ts.restartType,
		intervalMs:   ts.intervalMs,
		backoff:      ts.backoff,
		runnerLabels: runnerLabels,
	}
}

// RuntimeEquals reports whether two specs are identical in every field
// that `SpecToProto` reads — i.e. every field that affects what the
// agent actually runs. Returns `true` for pure-metadata edits (name,
// targets, targetLabels, timestamps), which must NOT rotate the
// generation counter.
//
// Fields compared: slot, kindType, kindConfig, timeoutMs, restartType,
// intervalMs, backoff, runnerLabels.
//
// Reason this lives in the domain package and not in the service: it's
// a pure property of the Spec value type, used by Upsert to decide
// whether to call `BumpGeneration`. Keeping the list of runtime fields
// next to the struct definition is the easiest way to keep it honest
// when someone adds a new field and forgets to update the comparator.
func (ts *Spec) RuntimeEquals(other *Spec) bool {
	if ts == nil || other == nil {
		return ts == other
	}
	if ts.slot != other.slot ||
		ts.kindType != other.kindType ||
		ts.timeoutMs != other.timeoutMs ||
		ts.restartType != other.restartType ||
		ts.intervalMs != other.intervalMs ||
		ts.backoff != other.backoff {
		return false
	}
	if !stringMapEqual(ts.runnerLabels, other.runnerLabels) {
		return false
	}
	return anyMapEqual(ts.kindConfig, other.kindConfig)
}

// stringMapEqual returns true when both maps hold the same set of
// (key, value) pairs.
func stringMapEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// anyMapEqual compares two `map[string]any`. Nested values fall back to
// `reflect.DeepEqual` because kindConfig is user-supplied JSON — there is
// no richer type information to exploit. Called only on Upsert, not in a
// hot path.
func anyMapEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok || !reflect.DeepEqual(v, bv) {
			return false
		}
	}
	return true
}
