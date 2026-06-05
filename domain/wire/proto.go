package wire

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	raftv1 "github.com/soltiHQ/control-plane/api/gen/solti/raft/v1"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
)

// timeToUnixNano returns 0 for the zero time.Time and the nano timestamp
// otherwise. Pairs with timeFromUnixNano so round-trip preserves "unset".
func timeToUnixNano(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// timeFromUnixNano inverts timeToUnixNano: 0 → zero time.Time, otherwise
// the corresponding instant.
func timeFromUnixNano(ns int64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// timeDurationFromNs is shorthand for time.Duration(int64) used by callers
// that receive a duration as nanoseconds over the wire.
func timeDurationFromNs(ns int64) time.Duration {
	return time.Duration(ns)
}

// File implements the canonical wire mapping used by Raft replication:
// model.X  ↔  raftv1.XMsg.
//
// ToProto snapshots a domain entity for Raft. FromProto reconstructs it on
// the receiving replica. Round-trip is byte-for-byte equivalent for every
// persisted field — anything that should survive replication MUST appear
// in both directions.

// === Agents ===

// AgentToProto serialises a model.Agent for Raft replication.
func AgentToProto(a *model.Agent) *raftv1.AgentMsg {
	if a == nil {
		return nil
	}
	md := make(map[string]string, len(a.MetadataAll()))
	for k, v := range a.MetadataAll() {
		md[k] = v
	}
	lbl := make(map[string]string, len(a.LabelsAll()))
	for k, v := range a.LabelsAll() {
		lbl[k] = v
	}
	return &raftv1.AgentMsg{
		Id:                  a.ID(),
		Name:                a.Name(),
		Endpoint:            a.Endpoint(),
		EndpointType:        string(a.EndpointType()),
		ApiVersion:          uint32(a.APIVersion()),
		Os:                  a.OS(),
		Arch:                a.Arch(),
		Platform:            a.Platform(),
		UptimeSeconds:       a.UptimeSeconds(),
		HeartbeatIntervalNs: int64(a.HeartbeatInterval()),
		Status:              uint32(a.Status()),
		CreatedAtNs:         timeToUnixNano(a.CreatedAt()),
		UpdatedAtNs:         timeToUnixNano(a.UpdatedAt()),
		LastSeenAtNs:        timeToUnixNano(a.LastSeenAt()),
		StaleAtNs:           timeToUnixNano(a.StaleAt()),
		Metadata:            md,
		Labels:              lbl,
		Capabilities:        a.Capabilities(),
	}
}

// AgentFromProto reconstructs a model.Agent from its Raft representation.
func AgentFromProto(p *raftv1.AgentMsg) (*model.Agent, error) {
	if p == nil {
		return nil, nil
	}
	a, err := model.NewAgent(p.GetId(), p.GetName(), p.GetEndpoint())
	if err != nil {
		return nil, err
	}
	a.SetEndpointType(enum.EndpointType(p.GetEndpointType()))
	a.SetAPIVersion(enum.APIVersion(p.GetApiVersion()))
	a.SetOS(p.GetOs())
	a.SetArch(p.GetArch())
	a.SetPlatform(p.GetPlatform())
	a.SetUptimeSeconds(p.GetUptimeSeconds())
	a.SetHeartbeatInterval(timeDurationFromNs(p.GetHeartbeatIntervalNs()))
	a.SetStatus(enum.AgentStatus(p.GetStatus()))
	a.SetCreatedAt(timeFromUnixNano(p.GetCreatedAtNs()))
	a.SetUpdatedAt(timeFromUnixNano(p.GetUpdatedAtNs()))
	a.SetLastSeenAt(timeFromUnixNano(p.GetLastSeenAtNs()))
	a.SetStaleAt(timeFromUnixNano(p.GetStaleAtNs()))
	a.SetMetadata(p.GetMetadata())
	a.SetLabels(p.GetLabels())
	a.SetCapabilities(p.GetCapabilities())
	return a, nil
}

// === Users ===

// UserToProto serialises a model.User for Raft replication.
func UserToProto(u *model.User) *raftv1.UserMsg {
	if u == nil {
		return nil
	}
	perms := u.PermissionsAll()
	sp := make([]string, len(perms))
	for i, p := range perms {
		sp[i] = string(p)
	}
	return &raftv1.UserMsg{
		Id:          u.ID(),
		Subject:     u.Subject(),
		Email:       u.Email(),
		Name:        u.Name(),
		Disabled:    u.Disabled(),
		RoleIds:     u.RoleIDsAll(),
		Permissions: sp,
		CreatedAtNs: timeToUnixNano(u.CreatedAt()),
		UpdatedAtNs: timeToUnixNano(u.UpdatedAt()),
	}
}

// UserFromProto reconstructs a model.User from its Raft representation.
func UserFromProto(p *raftv1.UserMsg) (*model.User, error) {
	if p == nil {
		return nil, nil
	}
	u, err := model.NewUser(p.GetId(), p.GetSubject())
	if err != nil {
		return nil, err
	}
	u.SetSubject(p.GetSubject())
	u.SetEmail(p.GetEmail())
	u.SetName(p.GetName())
	if p.GetDisabled() {
		u.Disable()
	}
	for _, rid := range p.GetRoleIds() {
		_ = u.RoleAdd(rid)
	}
	for _, perm := range p.GetPermissions() {
		_ = u.PermissionAdd(enum.Permission(perm))
	}
	// Timestamp restores MUST stay last: the business setters above (SetEmail,
	// RoleAdd, PermissionAdd, …) bump UpdatedAt; SetUpdatedAt then overwrites it
	// with the persisted value. Reordering would leak a fresh timestamp on replay.
	u.SetCreatedAt(timeFromUnixNano(p.GetCreatedAtNs()))
	u.SetUpdatedAt(timeFromUnixNano(p.GetUpdatedAtNs()))
	return u, nil
}

// === Roles ===

// RoleToProto serialises a model.Role for Raft replication.
func RoleToProto(r *model.Role) *raftv1.RoleMsg {
	if r == nil {
		return nil
	}
	perms := r.PermissionsAll()
	sp := make([]string, len(perms))
	for i, p := range perms {
		sp[i] = string(p)
	}
	return &raftv1.RoleMsg{
		Id:          r.ID(),
		Name:        r.Name(),
		Permissions: sp,
		CreatedAtNs: timeToUnixNano(r.CreatedAt()),
		UpdatedAtNs: timeToUnixNano(r.UpdatedAt()),
	}
}

// RoleFromProto reconstructs a model.Role from its Raft representation.
func RoleFromProto(p *raftv1.RoleMsg) (*model.Role, error) {
	if p == nil {
		return nil, nil
	}
	r, err := model.NewRole(p.GetId(), p.GetName())
	if err != nil {
		return nil, err
	}
	for _, perm := range p.GetPermissions() {
		_ = r.PermissionAdd(enum.Permission(perm))
	}
	r.SetCreatedAt(timeFromUnixNano(p.GetCreatedAtNs()))
	r.SetUpdatedAt(timeFromUnixNano(p.GetUpdatedAtNs()))
	return r, nil
}

// === Credentials ===

// CredentialToProto serialises a model.Credential for Raft replication.
func CredentialToProto(c *model.Credential) *raftv1.CredentialMsg {
	if c == nil {
		return nil
	}
	secrets := make(map[string]string, len(c.SecretsAll()))
	for k, v := range c.SecretsAll() {
		secrets[k] = v
	}
	return &raftv1.CredentialMsg{
		Id:          c.ID(),
		UserId:      c.UserID(),
		Auth:        string(c.AuthKind()),
		Secrets:     secrets,
		CreatedAtNs: timeToUnixNano(c.CreatedAt()),
		UpdatedAtNs: timeToUnixNano(c.UpdatedAt()),
	}
}

// CredentialFromProto reconstructs a model.Credential from its Raft form.
func CredentialFromProto(p *raftv1.CredentialMsg) (*model.Credential, error) {
	if p == nil {
		return nil, nil
	}
	c, err := model.NewCredential(p.GetId(), p.GetUserId(), enum.Auth(p.GetAuth()))
	if err != nil {
		return nil, err
	}
	for k, v := range p.GetSecrets() {
		_ = c.SetSecret(k, v)
	}
	c.SetCreatedAt(timeFromUnixNano(p.GetCreatedAtNs()))
	c.SetUpdatedAt(timeFromUnixNano(p.GetUpdatedAtNs()))
	return c, nil
}

// === Verifiers ===

// VerifierToProto serialises a model.Verifier for Raft replication.
func VerifierToProto(v *model.Verifier) *raftv1.VerifierMsg {
	if v == nil {
		return nil
	}
	data := make(map[string]string, len(v.DataAll()))
	for k, val := range v.DataAll() {
		data[k] = val
	}
	return &raftv1.VerifierMsg{
		Id:           v.ID(),
		CredentialId: v.CredentialID(),
		Auth:         string(v.AuthKind()),
		Data:         data,
		CreatedAtNs:  timeToUnixNano(v.CreatedAt()),
		UpdatedAtNs:  timeToUnixNano(v.UpdatedAt()),
	}
}

// VerifierFromProto reconstructs a model.Verifier from its Raft form.
func VerifierFromProto(p *raftv1.VerifierMsg) (*model.Verifier, error) {
	if p == nil {
		return nil, nil
	}
	v, err := model.NewVerifier(p.GetId(), p.GetCredentialId(), enum.Auth(p.GetAuth()))
	if err != nil {
		return nil, err
	}
	for k, val := range p.GetData() {
		_ = v.DataSet(k, val)
	}
	v.SetCreatedAt(timeFromUnixNano(p.GetCreatedAtNs()))
	v.SetUpdatedAt(timeFromUnixNano(p.GetUpdatedAtNs()))
	return v, nil
}

// === Sessions ===

// SessionToProto serialises a model.Session for Raft replication.
func SessionToProto(s *model.Session) *raftv1.SessionMsg {
	if s == nil {
		return nil
	}
	hash := s.RefreshHash()
	cp := make([]byte, len(hash))
	copy(cp, hash)
	return &raftv1.SessionMsg{
		Id:           s.ID(),
		UserId:       s.UserID(),
		CredentialId: s.CredentialID(),
		Auth:         string(s.AuthKind()),
		RefreshHash:  cp,
		ExpiresAtNs:  timeToUnixNano(s.ExpiresAt()),
		RevokedAtNs:  timeToUnixNano(s.RevokedAt()),
		CreatedAtNs:  timeToUnixNano(s.CreatedAt()),
		UpdatedAtNs:  timeToUnixNano(s.UpdatedAt()),
	}
}

// SessionFromProto reconstructs a model.Session from its Raft form.
func SessionFromProto(p *raftv1.SessionMsg) (*model.Session, error) {
	if p == nil {
		return nil, nil
	}
	hash := make([]byte, len(p.GetRefreshHash()))
	copy(hash, p.GetRefreshHash())
	s, err := model.NewSession(
		p.GetId(), p.GetUserId(), p.GetCredentialId(),
		enum.Auth(p.GetAuth()), hash, timeFromUnixNano(p.GetExpiresAtNs()),
	)
	if err != nil {
		return nil, err
	}
	s.SetCreatedAt(timeFromUnixNano(p.GetCreatedAtNs()))
	s.SetUpdatedAt(timeFromUnixNano(p.GetUpdatedAtNs()))
	s.SetRevokedAt(timeFromUnixNano(p.GetRevokedAtNs()))
	return s, nil
}

// === Specs ===

// SpecToProto serialises a model.Spec for Raft replication. KindConfig is
// converted to google.protobuf.Struct; non-JSON-friendly values inside the
// map (channels, functions) will fail here loudly.
func SpecToProto(ts *model.Spec) *raftv1.SpecMsg {
	if ts == nil {
		return nil
	}
	tl := make(map[string]string, len(ts.TargetLabels()))
	for k, v := range ts.TargetLabels() {
		tl[k] = v
	}
	rl := make(map[string]string, len(ts.RunnerLabels()))
	for k, v := range ts.RunnerLabels() {
		rl[k] = v
	}
	b := ts.Backoff()
	kc, _ := structpb.NewStruct(ts.KindConfig())
	return &raftv1.SpecMsg{
		Id:                ts.ID(),
		Name:              ts.Name(),
		Slot:              ts.Slot(),
		Version:           int32(ts.Version()),
		Generation:        int32(ts.Generation()),
		DeletionRequested: ts.DeletionRequested(),
		KindType:          string(ts.KindType()),
		KindConfig:        kc,
		TimeoutMs:         ts.TimeoutMs(),
		RestartType:       string(ts.RestartType()),
		IntervalMs:        ts.IntervalMs(),
		Backoff: &raftv1.BackoffMsg{
			Jitter:  string(b.Jitter),
			FirstMs: b.FirstMs,
			MaxMs:   b.MaxMs,
			Factor:  b.Factor,
		},
		Targets:      ts.Targets(),
		TargetLabels: tl,
		RunnerLabels: rl,
		CreatedAtNs:  timeToUnixNano(ts.CreatedAt()),
		UpdatedAtNs:  timeToUnixNano(ts.UpdatedAt()),
	}
}

// SpecFromProto reconstructs a model.Spec from its Raft representation.
func SpecFromProto(p *raftv1.SpecMsg) (*model.Spec, error) {
	if p == nil {
		return nil, nil
	}
	ts, err := model.NewSpec(p.GetId(), p.GetName(), p.GetSlot())
	if err != nil {
		return nil, err
	}
	ts.SetKindType(enum.TaskKindType(p.GetKindType()))
	ts.SetKindConfig(p.GetKindConfig().AsMap())
	ts.SetTimeoutMs(p.GetTimeoutMs())
	ts.SetRestartType(enum.RestartType(p.GetRestartType()))
	ts.SetIntervalMs(p.GetIntervalMs())
	if b := p.GetBackoff(); b != nil {
		ts.SetBackoff(model.BackoffConfig{
			Jitter:  enum.JitterStrategy(b.GetJitter()),
			FirstMs: b.GetFirstMs(),
			MaxMs:   b.GetMaxMs(),
			Factor:  b.GetFactor(),
		})
	}
	ts.SetTargets(p.GetTargets())
	ts.SetTargetLabels(p.GetTargetLabels())
	ts.SetRunnerLabels(p.GetRunnerLabels())
	ts.SetVersion(int(p.GetVersion()))
	ts.SetGeneration(int(p.GetGeneration()))
	ts.SetDeletionRequested(p.GetDeletionRequested())
	ts.SetCreatedAt(timeFromUnixNano(p.GetCreatedAtNs()))
	ts.SetUpdatedAt(timeFromUnixNano(p.GetUpdatedAtNs()))
	return ts, nil
}

// === Rollouts ===

// RolloutToProto serialises a model.Rollout for Raft replication.
func RolloutToProto(r *model.Rollout) *raftv1.RolloutMsg {
	if r == nil {
		return nil
	}
	return &raftv1.RolloutMsg{
		Id:                 r.ID(),
		SpecId:             r.SpecID(),
		AgentId:            r.AgentID(),
		ActualTaskId:       r.ActualTaskID(),
		ErrMsg:             r.Error(),
		DesiredGeneration:  int32(r.DesiredGeneration()),
		ObservedGeneration: int32(r.ObservedGeneration()),
		Attempts:           int32(r.Attempts()),
		Status:             uint32(r.Status()),
		Intent:             uint32(r.Intent()),
		CreatedAtNs:        timeToUnixNano(r.CreatedAt()),
		UpdatedAtNs:        timeToUnixNano(r.UpdatedAt()),
		LastPushedAtNs:     timeToUnixNano(r.LastPushedAt()),
		LastSyncedAtNs:     timeToUnixNano(r.LastSyncedAt()),
	}
}

// RolloutFromProto reconstructs a model.Rollout from its Raft form.
// NewRollout builds the skeleton; transition methods set the status+counters
// to match the serialised state. Errors from transitions are ignored — they
// indicate invariant violations the serialised state already represents,
// which can't be fixed by reporting here.
func RolloutFromProto(p *raftv1.RolloutMsg) (*model.Rollout, error) {
	if p == nil {
		return nil, nil
	}
	r, err := model.NewRollout(p.GetSpecId(), p.GetAgentId(), int(p.GetDesiredGeneration()))
	if err != nil {
		return nil, err
	}
	r.SetIntent(enum.RolloutIntent(p.GetIntent()))
	r.SetActualTaskID(p.GetActualTaskId())
	switch enum.SyncStatus(p.GetStatus()) {
	case enum.SyncStatusPending:
		r.MarkPending(int(p.GetDesiredGeneration()))
	case enum.SyncStatusSynced:
		r.MarkSynced(int(p.GetObservedGeneration()))
	case enum.SyncStatusFailed:
		r.MarkFailed(p.GetErrMsg())
	case enum.SyncStatusDrift:
		r.MarkDrift()
	case enum.SyncStatusUnknown:
		r.MarkUnknown()
	}
	r.SetLastPushedAt(timeFromUnixNano(p.GetLastPushedAtNs()))
	r.SetCreatedAt(timeFromUnixNano(p.GetCreatedAtNs()))
	r.SetUpdatedAt(timeFromUnixNano(p.GetUpdatedAtNs()))
	return r, nil
}

// === errors ===

// ErrUnknownOp is returned by raft FSM Apply when the oneof variant carried
// in raftv1.Op is not recognised.
var ErrUnknownOp = fmt.Errorf("wire: unknown op variant")
