// Package proxyv1 defines the REST/JSON shapes the outbound proxy exposes to the UI.
//
// Agents talk over gRPC using the protobuf types in solti.task.v1
// (generated under api/gen/solti/task/v1).
//
// The proxy converts those into the structs here, so the UI does not depend on the proto schema.
//
// These types are also the return contract of proxy.AgentProxy.
package proxyv1
