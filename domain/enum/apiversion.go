package enum

// APIVersion describes the agent API version the control-plane should use when communicating with the agent.
type APIVersion uint8

const (
	APIVersionUnspecified APIVersion = iota
	APIVersionV1
)

// String returns the human-readable version label.
func (v APIVersion) String() string {
	switch v {
	case APIVersionV1:
		return "v1"
	default:
		return "unknown"
	}
}

// APIVersionFromInt maps the agent-reported integer API version to APIVersion.
//
//	1 → v1. Any other value (including 0) falls back to Unspecified.
//
// Wire source: api_version in solti.discover.v1 ("1 = v1") and solti.raft.v1.AgentMsg.
func APIVersionFromInt(v int) APIVersion {
	switch v {
	case 1:
		return APIVersionV1
	default:
		return APIVersionUnspecified
	}
}
