package proxy

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	taskv1 "github.com/soltiHQ/control-plane/api/gen/solti/task/v1"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
)

// SpecToProto converts a domain Spec into the generated proto CreateSpec.
//
// The returned value is marshaled to the wire via protojson (canonical
// proto-JSON: camelCase + enum-as-string + uint64-as-string) on the HTTP
// path, and passed directly into gRPC calls on the gRPC path — so both
// transports share a single source of truth defined in
// api/proto/v1/types.proto.
//
// Errors:
//   - ts == nil
//   - unknown TaskKindType
//   - the UI-supplied KindConfig cannot be round-tripped into proto
//     (invalid field names or value types — see solti.task.v1.TaskKind).
func SpecToProto(ts *model.Spec) (*taskv1.CreateSpec, error) {
	if ts == nil {
		return nil, fmt.Errorf("proxy: nil spec")
	}

	tk, err := buildTaskKind(ts.KindType(), ts.KindConfig())
	if err != nil {
		return nil, fmt.Errorf("proxy: task kind: %w", err)
	}

	b := ts.Backoff()
	out := &taskv1.CreateSpec{
		Slot:      ts.Slot(),
		Kind:      tk,
		TimeoutMs: clampU64(ts.TimeoutMs()),
		Restart:   restartStrategyToProto(ts.RestartType()),
		Backoff: &taskv1.BackoffStrategy{
			Jitter:  jitterStrategyToProto(b.Jitter),
			FirstMs: clampU64(b.FirstMs),
			MaxMs:   clampU64(b.MaxMs),
			Factor:  b.Factor,
		},
		// Admission is CP-managed. The sync runner upgrades via ApplyTask,
		// which force-replaces at the agent regardless of this field — so a
		// new spec version always wins the slot. We still pin Replace here so
		// the value matches the effective behaviour (e.g. in the wire preview);
		// the UI doesn't expose the field.
		Admission: taskv1.AdmissionStrategy_ADMISSION_STRATEGY_REPLACE,
	}

	if ts.RestartType() == enum.RestartAlways && ts.IntervalMs() > 0 {
		iv := uint64(ts.IntervalMs())
		out.RestartIntervalMs = &iv
	}

	if labels := ts.RunnerLabels(); len(labels) > 0 {
		out.Labels = labels
	}

	return out, nil
}

// buildTaskKind re-serializes the UI-stored kind config map through protojson
// so the UI-level JSON (which is expected to already follow proto schema
// field names) lands in the strongly-typed generated struct without
// per-field manual mapping.
//
// The input `cfg` must already be canonical proto3-JSON for the payload of
// the selected TaskKind variant. For `subprocess`, that means the `oneof mode`
// is inlined (`{"command":{...}}` or `{"script":{...}}`, not
// `{"mode":{"command":{...}}}`) and `env` is `repeated KeyValue`
// (`[{"key","value"}]`, not a map).
//
// DiscardUnknown is kept on so forward-compatible extensions of the proto
// schema don't hard-fail old control-planes. That, however, means silently
// misnamed fields (`{"mode":{...}}`) get dropped. We therefore run a
// post-decode sanity check that raises a loud error instead of handing a
// half-populated TaskKind to the agent.
func buildTaskKind(kt enum.TaskKindType, cfg map[string]any) (*taskv1.TaskKind, error) {
	name, err := taskKindName(kt)
	if err != nil {
		return nil, err
	}
	// Wrap into a oneof shell: {"subprocess": {...}}, {"container": {...}}, {"wasm": {...}}.
	payload, err := json.Marshal(map[string]any{name: cfg})
	if err != nil {
		return nil, fmt.Errorf("marshal kind config: %w", err)
	}
	var tk taskv1.TaskKind
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(payload, &tk); err != nil {
		return nil, fmt.Errorf("protojson decode: %w", err)
	}
	if err := sanityCheckTaskKind(&tk, kt, cfg); err != nil {
		return nil, err
	}
	return &tk, nil
}

// sanityCheckTaskKind surfaces silent data loss caused by DiscardUnknown: if
// protojson dropped the entire oneof payload (the common symptom is a caller
// that wrapped it in an extra envelope field), we report exactly which key
// the caller supplied that we could not map onto the proto schema.
func sanityCheckTaskKind(tk *taskv1.TaskKind, kt enum.TaskKindType, cfg map[string]any) error {
	if tk.GetKind() == nil {
		return fmt.Errorf("proxy: TaskKind.kind unset after decode for %q (likely bad JSON shape)", kt)
	}
	switch kt {
	case enum.TaskKindSubprocess:
		sub := tk.GetSubprocess()
		if sub == nil {
			return fmt.Errorf("proxy: SubprocessTask unset after decode")
		}
		if sub.GetMode() == nil {
			// Most common cause: UI wraps the oneof in an extra "mode" field.
			// Canonical proto3-JSON inlines the oneof directly.
			for key := range cfg {
				switch key {
				case "command", "script", "env", "cwd", "failOnNonZero", "fail_on_non_zero":
					// Known top-level SubprocessTask fields, not suspicious.
				default:
					return fmt.Errorf("proxy: SubprocessTask.mode unset after decode; "+
						"unrecognized top-level key %q in subprocess config "+
						"(oneof must be inlined: {\"command\":{...}} or {\"script\":{...}})", key)
				}
			}
			return fmt.Errorf("proxy: SubprocessTask.mode unset after decode " +
				"(neither \"command\" nor \"script\" found at top level)")
		}
	case enum.TaskKindWasm:
		if tk.GetWasm() == nil {
			return fmt.Errorf("proxy: WasmTask unset after decode")
		}
	case enum.TaskKindContainer:
		if tk.GetContainer() == nil {
			return fmt.Errorf("proxy: ContainerTask unset after decode")
		}
	}
	return nil
}

func taskKindName(kt enum.TaskKindType) (string, error) {
	switch kt {
	case enum.TaskKindSubprocess:
		return "subprocess", nil
	case enum.TaskKindContainer:
		return "container", nil
	case enum.TaskKindWasm:
		return "wasm", nil
	default:
		return "", fmt.Errorf("unknown task kind: %q", kt)
	}
}

func restartStrategyToProto(rt enum.RestartType) taskv1.RestartStrategy {
	switch rt {
	case enum.RestartNever:
		return taskv1.RestartStrategy_RESTART_STRATEGY_NEVER
	case enum.RestartOnFailure:
		return taskv1.RestartStrategy_RESTART_STRATEGY_ON_FAILURE
	case enum.RestartAlways:
		return taskv1.RestartStrategy_RESTART_STRATEGY_ALWAYS
	default:
		return taskv1.RestartStrategy_RESTART_STRATEGY_UNSPECIFIED
	}
}

func jitterStrategyToProto(j enum.JitterStrategy) taskv1.JitterStrategy {
	switch j {
	case enum.JitterNone:
		return taskv1.JitterStrategy_JITTER_STRATEGY_NONE
	case enum.JitterFull:
		return taskv1.JitterStrategy_JITTER_STRATEGY_FULL
	case enum.JitterEqual:
		return taskv1.JitterStrategy_JITTER_STRATEGY_EQUAL
	case enum.JitterDecorrelated:
		return taskv1.JitterStrategy_JITTER_STRATEGY_DECORRELATED
	default:
		return taskv1.JitterStrategy_JITTER_STRATEGY_UNSPECIFIED
	}
}

func clampU64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}
