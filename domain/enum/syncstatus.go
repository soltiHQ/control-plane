package enum

// SyncStatus describes the synchronization state of a Spec on a specific agent.
type SyncStatus uint8

const (
	SyncStatusPending SyncStatus = iota
	SyncStatusSynced
	SyncStatusDrift
	SyncStatusFailed
	SyncStatusUnknown
)

// String returns the human-readable sync status label.
func (s SyncStatus) String() string {
	switch s {
	case SyncStatusPending:
		return "pending"
	case SyncStatusFailed:
		return "failed"
	case SyncStatusSynced:
		return "synced"
	case SyncStatusDrift:
		return "drift"
	case SyncStatusUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}
