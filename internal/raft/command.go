package raft

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	raftv1 "github.com/soltiHQ/control-plane/api/gen/solti/raft/v1"
)

// encodeCommand serialises one Raft command for log submission. Wraps
// proto.Marshal so callers do not import the proto package.
func encodeCommand(ops []*raftv1.Op) ([]byte, error) {
	data, err := proto.Marshal(&raftv1.Command{Ops: ops})
	if err != nil {
		return nil, fmt.Errorf("raft: encode command: %w", err)
	}
	return data, nil
}

// decodeCommand inverts encodeCommand.
func decodeCommand(data []byte) (*raftv1.Command, error) {
	var cmd raftv1.Command
	if err := proto.Unmarshal(data, &cmd); err != nil {
		return nil, fmt.Errorf("raft: decode command: %w", err)
	}
	return &cmd, nil
}
