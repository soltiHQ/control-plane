// Package wire holds canonical wire conversions for domain entities.
//
// Two pathways:
//   - Raft replication & FSM snapshots — model.X ↔ genv1.XMsg (proto, see proto.go)
//   - REST API responses               — model.X →   restv1.X (see rest.go)
//
// ToProto / FromProto round-trip via domain constructors + persistence setters
// to reconstruct the entity byte-for-byte on the receiving replica.
package wire
