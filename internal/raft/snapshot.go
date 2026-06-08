package raft

import (
	"fmt"

	hraft "github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"

	raftv1 "github.com/soltiHQ/control-plane/api/gen/solti/raft/v1"
	"github.com/soltiHQ/control-plane/domain/wire"
	"github.com/soltiHQ/control-plane/internal/storage/inmemory"
)

const (
	snapshotMagic   = "SOLTI-SNAP"
	snapshotVersion = uint32(1)
)

// fsmSnapshot is the hraft.FSMSnapshot returned by FSM.Snapshot.
//
// It captures a decoupled copy of the store contents at Snapshot() time
// so that Persist() can run concurrently with later Apply calls without
// observing partial mutations. Persist serialises the captured payload
// as a single proto blob (raftv1.Snapshot) into the SnapshotSink.
type fsmSnapshot struct {
	content inmemory.SnapshotContent
}

// Persist marshals the captured state into sink as one proto message.
// On any encode/write error Persist cancels the sink (telling Raft to
// discard the partial file) and returns the wrapped error.
func (s *fsmSnapshot) Persist(sink hraft.SnapshotSink) error {
	msg := &raftv1.Snapshot{
		Header: &raftv1.SnapshotHeader{
			Magic:   snapshotMagic,
			Version: snapshotVersion,
		},
		Agents:           make([]*raftv1.AgentMsg, 0, len(s.content.Agents)),
		AgentCredentials: make([]*raftv1.AgentCredentialMsg, 0, len(s.content.AgentCreds)),
		Users:            make([]*raftv1.UserMsg, 0, len(s.content.Users)),
		Roles:            make([]*raftv1.RoleMsg, 0, len(s.content.Roles)),
		Credentials:      make([]*raftv1.CredentialMsg, 0, len(s.content.Credentials)),
		Verifiers:        make([]*raftv1.VerifierMsg, 0, len(s.content.Verifiers)),
		Sessions:         make([]*raftv1.SessionMsg, 0, len(s.content.Sessions)),
		Specs:            make([]*raftv1.SpecMsg, 0, len(s.content.Specs)),
		Rollouts:         make([]*raftv1.RolloutMsg, 0, len(s.content.Rollouts)),
	}
	for _, a := range s.content.Agents {
		msg.Agents = append(msg.Agents, wire.AgentToProto(a))
	}
	for _, c := range s.content.AgentCreds {
		msg.AgentCredentials = append(msg.AgentCredentials, wire.AgentCredentialToProto(c))
	}
	for _, u := range s.content.Users {
		msg.Users = append(msg.Users, wire.UserToProto(u))
	}
	for _, r := range s.content.Roles {
		msg.Roles = append(msg.Roles, wire.RoleToProto(r))
	}
	for _, c := range s.content.Credentials {
		msg.Credentials = append(msg.Credentials, wire.CredentialToProto(c))
	}
	for _, v := range s.content.Verifiers {
		msg.Verifiers = append(msg.Verifiers, wire.VerifierToProto(v))
	}
	for _, ss := range s.content.Sessions {
		msg.Sessions = append(msg.Sessions, wire.SessionToProto(ss))
	}
	for _, sp := range s.content.Specs {
		msg.Specs = append(msg.Specs, wire.SpecToProto(sp))
	}
	for _, ro := range s.content.Rollouts {
		msg.Rollouts = append(msg.Rollouts, wire.RolloutToProto(ro))
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		_ = sink.Cancel()
		return fmt.Errorf("raft snapshot: marshal: %w", err)
	}
	if _, err := sink.Write(data); err != nil {
		_ = sink.Cancel()
		return fmt.Errorf("raft snapshot: write: %w", err)
	}
	return sink.Close()
}

// Release is part of the hraft.FSMSnapshot interface; nothing to release.
func (s *fsmSnapshot) Release() {}
