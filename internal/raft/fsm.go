package raft

import (
	"context"
	"fmt"
	"io"
	"time"

	hraft "github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"

	genv1 "github.com/soltiHQ/control-plane/api/gen/v1"
	"github.com/soltiHQ/control-plane/domain/wire"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/storage/inmemory"
)

// Compile-time check.
var _ hraft.FSM = (*FSM)(nil)

// FSM applies replicated commands to the underlying storage and hub.
//
// Store mutations run inside one WithTx per command (all-or-nothing).
// Event ops (Notify/Record/DeleteIssues) are applied to the hub outside
// the transaction — they are append-only and have no rollback semantics.
// The FSM runs on every replica on every committed log entry.
type FSM struct {
	store storage.Storage
	hub   *event.Hub
}

// NewFSM builds an FSM wrapping store and hub. store must be the plain
// inmemory one (NOT a Raft-backed wrapper) to avoid recursive Apply; hub
// must be the local *event.Hub whose ApplyLocal* methods we call directly.
func NewFSM(store storage.Storage, hub *event.Hub) *FSM {
	return &FSM{store: store, hub: hub}
}

// Apply decodes the proto Command, runs store ops under one WithTx, and
// fires event ops on the local hub afterwards.
func (f *FSM) Apply(l *hraft.Log) any {
	cmd, err := decodeCommand(l.Data)
	if err != nil {
		return err
	}
	ctx := context.Background()

	// Split ops: store ops go through WithTx, event ops are applied
	// directly on the hub after the tx commits successfully.
	var (
		storeOps []*genv1.Op
		eventOps []*genv1.Op
	)
	for _, op := range cmd.GetOps() {
		if isEventOp(op) {
			eventOps = append(eventOps, op)
		} else {
			storeOps = append(storeOps, op)
		}
	}

	if len(storeOps) > 0 {
		err := f.store.WithTx(ctx, func(tx storage.Storage) error {
			for i, op := range storeOps {
				if err := applyStoreOp(ctx, tx, op); err != nil {
					return fmt.Errorf("op[%d] %T: %w", i, op.GetOp(), err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	for _, op := range eventOps {
		applyEventOp(f.hub, op)
	}
	return nil
}

// isEventOp reports whether op carries an event-hub mutation rather than
// a store mutation.
func isEventOp(op *genv1.Op) bool {
	switch op.GetOp().(type) {
	case *genv1.Op_EventNotify, *genv1.Op_EventRecord, *genv1.Op_EventDeleteIssues:
		return true
	}
	return false
}

// applyEventOp dispatches a hub mutation to the local replica.
func applyEventOp(hub *event.Hub, op *genv1.Op) {
	if hub == nil {
		return
	}
	switch v := op.GetOp().(type) {
	case *genv1.Op_EventNotify:
		hub.ApplyLocalNotify(v.EventNotify)
	case *genv1.Op_EventRecord:
		msg := v.EventRecord
		hub.ApplyLocalRecord(msg.GetKind(), payloadFromProto(msg.GetPayload()))
	case *genv1.Op_EventDeleteIssues:
		msg := v.EventDeleteIssues
		hub.ApplyLocalDeleteIssues(msg.GetKind(), msg.GetId())
	}
}

// applyStoreOp dispatches a store mutation to tx. The variant of op
// drives the type-switch; each case converts the proto sub-message via
// wire.*FromProto and calls the corresponding tx method.
func applyStoreOp(ctx context.Context, tx storage.Storage, op *genv1.Op) error {
	switch v := op.GetOp().(type) {
	case *genv1.Op_AgentUpsert:
		a, err := wire.AgentFromProto(v.AgentUpsert)
		if err != nil {
			return err
		}
		return tx.UpsertAgent(ctx, a)
	case *genv1.Op_AgentDelete:
		return tx.DeleteAgent(ctx, v.AgentDelete)

	case *genv1.Op_UserUpsert:
		u, err := wire.UserFromProto(v.UserUpsert)
		if err != nil {
			return err
		}
		return tx.UpsertUser(ctx, u)
	case *genv1.Op_UserDelete:
		return tx.DeleteUser(ctx, v.UserDelete)

	case *genv1.Op_RoleUpsert:
		r, err := wire.RoleFromProto(v.RoleUpsert)
		if err != nil {
			return err
		}
		return tx.UpsertRole(ctx, r)
	case *genv1.Op_RoleDelete:
		return tx.DeleteRole(ctx, v.RoleDelete)

	case *genv1.Op_CredentialUpsert:
		c, err := wire.CredentialFromProto(v.CredentialUpsert)
		if err != nil {
			return err
		}
		return tx.UpsertCredential(ctx, c)
	case *genv1.Op_CredentialDelete:
		return tx.DeleteCredential(ctx, v.CredentialDelete)

	case *genv1.Op_VerifierUpsert:
		ver, err := wire.VerifierFromProto(v.VerifierUpsert)
		if err != nil {
			return err
		}
		return tx.UpsertVerifier(ctx, ver)
	case *genv1.Op_VerifierDelete:
		return tx.DeleteVerifier(ctx, v.VerifierDelete)
	case *genv1.Op_VerifierDeleteByCred:
		return tx.DeleteVerifierByCredential(ctx, v.VerifierDeleteByCred)

	case *genv1.Op_SessionCreate:
		s, err := wire.SessionFromProto(v.SessionCreate)
		if err != nil {
			return err
		}
		return tx.CreateSession(ctx, s)
	case *genv1.Op_SessionDelete:
		return tx.DeleteSession(ctx, v.SessionDelete)
	case *genv1.Op_SessionDeleteByUser:
		return tx.DeleteSessionsByUser(ctx, v.SessionDeleteByUser)
	case *genv1.Op_SessionRotateRefresh:
		m := v.SessionRotateRefresh
		return tx.RotateRefresh(ctx, m.GetId(), m.GetRefreshHash(), time.Unix(0, m.GetExpiresAtNs()))
	case *genv1.Op_SessionRevoke:
		m := v.SessionRevoke
		return tx.RevokeSession(ctx, m.GetId(), time.Unix(0, m.GetRevokedAtNs()))

	case *genv1.Op_SpecUpsert:
		ts, err := wire.SpecFromProto(v.SpecUpsert)
		if err != nil {
			return err
		}
		return tx.UpsertSpec(ctx, ts)
	case *genv1.Op_SpecDelete:
		return tx.DeleteSpec(ctx, v.SpecDelete)

	case *genv1.Op_RolloutUpsert:
		r, err := wire.RolloutFromProto(v.RolloutUpsert)
		if err != nil {
			return err
		}
		return tx.UpsertRollout(ctx, r)
	case *genv1.Op_RolloutDelete:
		return tx.DeleteRollout(ctx, v.RolloutDelete)
	case *genv1.Op_RolloutDeleteBySpec:
		return tx.DeleteRolloutsBySpec(ctx, v.RolloutDeleteBySpec)

	default:
		return fmt.Errorf("%w: %T", wire.ErrUnknownOp, op.GetOp())
	}
}

// Snapshot captures the current store state for log compaction.
//
// The returned hraft.FSMSnapshot holds a decoupled copy of every entity
// (shallow-copied maps under each store's read lock, values are clones).
// Apply may run concurrently with the subsequent Persist call — the
// snapshot is not observed by later mutations.
//
// Currently only *inmemory.Store is supported; passing any other backend
// at construction time results in a descriptive error here.
func (f *FSM) Snapshot() (hraft.FSMSnapshot, error) {
	store, ok := f.store.(*inmemory.Store)
	if !ok {
		return nil, fmt.Errorf("raft snapshot: requires *inmemory.Store, got %T", f.store)
	}
	return &fsmSnapshot{content: store.SnapshotForRaft()}, nil
}

// Restore replaces the FSM state with the contents of r.
//
// The proto blob is fully decoded into memory before the live store is
// swapped, so a truncated or corrupted stream leaves the store untouched.
func (f *FSM) Restore(r io.ReadCloser) error {
	defer r.Close()

	store, ok := f.store.(*inmemory.Store)
	if !ok {
		return fmt.Errorf("raft restore: requires *inmemory.Store, got %T", f.store)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("raft restore: read: %w", err)
	}
	var snap genv1.Snapshot
	if err := proto.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("raft restore: unmarshal: %w", err)
	}

	hdr := snap.GetHeader()
	if hdr.GetMagic() != snapshotMagic {
		return fmt.Errorf("raft restore: bad magic %q (want %q)", hdr.GetMagic(), snapshotMagic)
	}
	if hdr.GetVersion() != snapshotVersion {
		return fmt.Errorf("raft restore: unsupported version %d (want %d)", hdr.GetVersion(), snapshotVersion)
	}

	content := inmemory.SnapshotContent{}
	for _, m := range snap.GetAgents() {
		a, err := wire.AgentFromProto(m)
		if err != nil {
			return fmt.Errorf("raft restore: agent: %w", err)
		}
		content.Agents = append(content.Agents, a)
	}
	for _, m := range snap.GetUsers() {
		u, err := wire.UserFromProto(m)
		if err != nil {
			return fmt.Errorf("raft restore: user: %w", err)
		}
		content.Users = append(content.Users, u)
	}
	for _, m := range snap.GetRoles() {
		ro, err := wire.RoleFromProto(m)
		if err != nil {
			return fmt.Errorf("raft restore: role: %w", err)
		}
		content.Roles = append(content.Roles, ro)
	}
	for _, m := range snap.GetCredentials() {
		c, err := wire.CredentialFromProto(m)
		if err != nil {
			return fmt.Errorf("raft restore: credential: %w", err)
		}
		content.Credentials = append(content.Credentials, c)
	}
	for _, m := range snap.GetVerifiers() {
		ver, err := wire.VerifierFromProto(m)
		if err != nil {
			return fmt.Errorf("raft restore: verifier: %w", err)
		}
		content.Verifiers = append(content.Verifiers, ver)
	}
	for _, m := range snap.GetSessions() {
		s, err := wire.SessionFromProto(m)
		if err != nil {
			return fmt.Errorf("raft restore: session: %w", err)
		}
		content.Sessions = append(content.Sessions, s)
	}
	for _, m := range snap.GetSpecs() {
		sp, err := wire.SpecFromProto(m)
		if err != nil {
			return fmt.Errorf("raft restore: spec: %w", err)
		}
		content.Specs = append(content.Specs, sp)
	}
	for _, m := range snap.GetRollouts() {
		ro, err := wire.RolloutFromProto(m)
		if err != nil {
			return fmt.Errorf("raft restore: rollout: %w", err)
		}
		content.Rollouts = append(content.Rollouts, ro)
	}

	store.RestoreFromSnapshot(content)
	return nil
}
