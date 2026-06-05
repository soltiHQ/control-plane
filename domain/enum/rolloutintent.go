package enum

// RolloutIntent describes what the sync runner should do on the next tick
// for a Rollout record.
//
// The split between Intent (what we want) and SyncStatus (what happened
// last time) mirrors the k8s reconciliation pattern: a controller observes
// desired and actual state independently, and the intent is a compact way
// to express "an action is still needed" even when status/version diffs do
// not capture the situation (e.g. an agent removed from targets — no
// version drift, yet an Uninstall is required).
type RolloutIntent uint8

const (
	// RolloutIntentNoop is the default for a freshly synced rollout. The
	// sync runner skips noop rollouts entirely; only external actions
	// (edit spec, redeploy, remove target) move a rollout out of Noop.
	RolloutIntentNoop RolloutIntent = iota

	// RolloutIntentInstall means the spec is present on this target
	// conceptually, but no Task has ever been installed on the agent yet
	// (ActualTaskID is empty). ApplyTask once, record the TaskId.
	RolloutIntentInstall

	// RolloutIntentUpdate means an earlier version of the spec is live on
	// the agent (ActualTaskID set) and the desired generation is newer.
	// The sync runner ApplyTasks the new spec, which supersedes the old.
	RolloutIntentUpdate

	// RolloutIntentUninstall means the rollout must go away from the
	// agent: either the agent was removed from spec.Targets, or the spec
	// itself was marked for deletion. After DeleteTask (or 404) the
	// rollout record itself is dropped from storage.
	RolloutIntentUninstall
)

// String returns the stable lower-case label used in REST payloads and
// tracing. Keep these values stable — the UI renders badges by exact
// match.
func (i RolloutIntent) String() string {
	switch i {
	case RolloutIntentInstall:
		return "install"
	case RolloutIntentUpdate:
		return "update"
	case RolloutIntentUninstall:
		return "uninstall"
	case RolloutIntentNoop:
		return "noop"
	default:
		return "noop"
	}
}
