package raft

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	genv1 "github.com/soltiHQ/control-plane/api/gen/v1"
)

// encodeCommand serialises one Raft command for log submission. Wraps
// proto.Marshal so callers do not import the proto package.
func encodeCommand(ops []*genv1.Op) ([]byte, error) {
	data, err := proto.Marshal(&genv1.Command{Ops: ops})
	if err != nil {
		return nil, fmt.Errorf("raft: encode command: %w", err)
	}
	return data, nil
}

// decodeCommand inverts encodeCommand.
func decodeCommand(data []byte) (*genv1.Command, error) {
	var cmd genv1.Command
	if err := proto.Unmarshal(data, &cmd); err != nil {
		return nil, fmt.Errorf("raft: decode command: %w", err)
	}
	return &cmd, nil
}
