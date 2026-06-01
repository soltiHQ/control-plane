package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/soltiHQ/control-plane/internal/transport/grpc/status"

	discoverv1 "github.com/soltiHQ/control-plane/api/gen/solti/discover/v1"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/service"
	"github.com/soltiHQ/control-plane/internal/service/agent"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/transport/http/responder"
	"github.com/soltiHQ/control-plane/internal/transport/http/response"
	"github.com/soltiHQ/control-plane/internal/transport/httpctx"
	"github.com/soltiHQ/control-plane/internal/uikit/htmx"
)

// discoverySyncUnmarshal is a protojson.UnmarshalOptions with
// DiscardUnknown=true so that forward-compatible extensions of SyncRequest
// from newer agents don't hard-fail old control-planes.
var discoverySyncUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// discoverySyncMarshal keeps the canonical proto-JSON contract: camelCase
// field names and enum-as-string, symmetric with the SDK's pbjson output.
var discoverySyncMarshal = protojson.MarshalOptions{
	UseProtoNames:   false,
	EmitUnpopulated: false,
}

// maxSyncBodyBytes caps SyncRequest bodies to defend against runaway clients.
// Matches the 256 KiB limit used by the SDK's HttpApi RequestBodyLimitLayer.
const maxSyncBodyBytes = 256 * 1024

// HTTPDiscovery handles agent discovery over HTTP.
type HTTPDiscovery struct {
	logger   zerolog.Logger
	agentSVC *agent.Service
	eventHub *event.Hub
}

// NewHTTPDiscovery creates a new HTTP discovery handler.
func NewHTTPDiscovery(logger zerolog.Logger, agentSVC *agent.Service, eventHub *event.Hub) *HTTPDiscovery {
	if agentSVC == nil {
		panic(service.ErrNilService)
	}
	if eventHub == nil {
		panic(event.ErrNilHub)
	}
	return &HTTPDiscovery{
		logger:   logger.With().Str("handler", "discovery-http").Logger(),
		agentSVC: agentSVC,
		eventHub: eventHub,
	}
}

// Sync handles POST /api/v1/discovery/sync.
//
// The request body is expected to be canonical proto-JSON (camelCase +
// enum-as-string) matching solti.discover.v1.SyncRequest. The SDK emits
// this format via pbjson; both HTTP and gRPC paths therefore share a single
// wire schema.
func (h *HTTPDiscovery) Sync(w http.ResponseWriter, r *http.Request) {
	mode := httpctx.ModeFromRequest(r)

	if r.Method != http.MethodPost {
		response.NotAllowed(w, r, mode)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxSyncBodyBytes))
	if err != nil {
		response.BadRequest(w, r, mode)
		return
	}

	var in discoverv1.SyncRequest
	if err := discoverySyncUnmarshal.Unmarshal(body, &in); err != nil {
		response.BadRequest(w, r, mode)
		return
	}

	a, err := model.NewAgentFrom(model.AgentParams{
		ID:                 in.GetId(),
		Name:               in.GetName(),
		Endpoint:           in.GetEndpoint(),
		EndpointType:       int(in.GetEndpointType()),
		APIVersion:         int(in.GetApiVersion()),
		OS:                 in.GetOs(),
		Arch:               in.GetArch(),
		Platform:           in.GetPlatform(),
		UptimeSeconds:      in.GetUptimeSeconds(),
		HeartbeatIntervalS: int(in.GetHeartbeatIntervalS()),
		Metadata:           in.GetMetadata(),
		Capabilities:       in.GetCapabilities(),
	})
	if err != nil {
		response.BadRequest(w, r, mode)
		return
	}

	existing, getErr := h.agentSVC.Get(r.Context(), in.GetId())
	if err = h.agentSVC.Upsert(r.Context(), a); err != nil {
		h.logger.Error().Err(err).Str("agent_id", in.GetId()).Msg("upsert failed")
		response.Unavailable(w, r, mode)
		return
	}
	switch {
	case errors.Is(getErr, storage.ErrNotFound):
		h.eventHub.Record(event.AgentConnected, event.Payload{ID: in.GetId(), Name: in.GetName(), By: "discovery"})
	case existing != nil && existing.Status() != enum.AgentStatusActive:
		h.eventHub.Record(event.AgentConnected, event.Payload{ID: in.GetId(), Name: in.GetName(), By: "discovery"})

		n := h.eventHub.DeleteIssues(event.AgentInactive, in.GetId())
		n += h.eventHub.DeleteIssues(event.AgentDisconnected, in.GetId())
		if n > 0 {
			h.eventHub.Record(event.IssueClosed, event.Payload{ID: in.GetId(), Name: in.GetName(), By: "discovery"})
			h.eventHub.Notify(htmx.DashboardUpdate)
		}
	}
	h.eventHub.Notify(htmx.AgentUpdate)

	resp := &discoverv1.SyncResponse{Success: true}
	respBytes, err := discoverySyncMarshal.Marshal(resp)
	if err != nil {
		h.logger.Error().Err(err).Msg("marshal SyncResponse")
		response.Unavailable(w, r, mode)
		return
	}
	response.OK(w, r, mode, &responder.View{RawJSON: respBytes})
}

// GRPCDiscovery implements discoverv1.DiscoverServiceServer.
type GRPCDiscovery struct {
	discoverv1.UnimplementedDiscoverServiceServer
	logger   zerolog.Logger
	agentSVC *agent.Service
	hub      *event.Hub
}

// NewGRPCDiscovery creates a new gRPC discovery handler.
func NewGRPCDiscovery(logger zerolog.Logger, agentSVC *agent.Service, hub *event.Hub) *GRPCDiscovery {
	if agentSVC == nil {
		panic(service.ErrNilService)
	}
	if hub == nil {
		panic(event.ErrNilHub)
	}
	return &GRPCDiscovery{
		logger:   logger.With().Str("handler", "discovery-grpc").Logger(),
		agentSVC: agentSVC,
		hub:      hub,
	}
}

// Sync implements discoverv1.DiscoverServiceServer.
func (g *GRPCDiscovery) Sync(ctx context.Context, req *discoverv1.SyncRequest) (*discoverv1.SyncResponse, error) {
	a, err := model.NewAgentFrom(model.AgentParams{
		ID:                 req.GetId(),
		Name:               req.GetName(),
		Endpoint:           req.GetEndpoint(),
		EndpointType:       int(req.GetEndpointType()),
		APIVersion:         int(req.GetApiVersion()),
		OS:                 req.GetOs(),
		Arch:               req.GetArch(),
		Platform:           req.GetPlatform(),
		UptimeSeconds:      req.GetUptimeSeconds(),
		HeartbeatIntervalS: int(req.GetHeartbeatIntervalS()),
		Metadata:           req.GetMetadata(),
		Capabilities:       req.GetCapabilities(),
	})
	if err != nil {
		return nil, status.Errorf(ctx, codes.InvalidArgument, "invalid agent data: %v", err)
	}

	existing, getErr := g.agentSVC.Get(ctx, req.GetId())
	if err = g.agentSVC.Upsert(ctx, a); err != nil {
		g.logger.Error().Err(err).Str("agent_id", req.GetId()).Msg("upsert failed")
		return nil, status.FromError(ctx, err).Err()
	}
	switch {
	case errors.Is(getErr, storage.ErrNotFound):
		g.hub.Record(event.AgentConnected, event.Payload{ID: req.GetId(), Name: req.GetName(), By: "discovery"})
	case existing != nil && existing.Status() != enum.AgentStatusActive:
		g.hub.Record(event.AgentConnected, event.Payload{ID: req.GetId(), Name: req.GetName(), By: "discovery"})

		n := g.hub.DeleteIssues(event.AgentInactive, req.GetId())
		n += g.hub.DeleteIssues(event.AgentDisconnected, req.GetId())
		if n > 0 {
			g.hub.Record(event.IssueClosed, event.Payload{ID: req.GetId(), Name: req.GetName(), By: "discovery"})
			g.hub.Notify(htmx.DashboardUpdate)
		}
	}
	g.hub.Notify(htmx.AgentUpdate)
	return &discoverv1.SyncResponse{Success: true}, nil
}
