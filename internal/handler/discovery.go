package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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
	logger      zerolog.Logger
	agentSVC    *agent.Service
	eventHub    *event.Hub
	requireAuth bool
}

// NewHTTPDiscovery creates a new HTTP discovery handler.
//
// requireAuth turns on per-agent bearer-token authentication (TOFU enroll +
// constant-time verify) on every sync.
func NewHTTPDiscovery(logger zerolog.Logger, agentSVC *agent.Service, eventHub *event.Hub, requireAuth bool) *HTTPDiscovery {
	if agentSVC == nil {
		panic(service.ErrNilService)
	}
	if eventHub == nil {
		panic(event.ErrNilHub)
	}
	return &HTTPDiscovery{
		logger:      logger.With().Str("handler", "discovery-http").Logger(),
		agentSVC:    agentSVC,
		eventHub:    eventHub,
		requireAuth: requireAuth,
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

	a, err := agentFromSyncRequest(&in)
	if err != nil {
		response.BadRequest(w, r, mode)
		return
	}

	if h.requireAuth {
		token := bearerToken(r.Header.Get("Authorization"))
		if err := h.agentSVC.VerifyOrEnrollToken(r.Context(), in.GetId(), token); err != nil {
			if errors.Is(err, agent.ErrUnauthenticated) {
				response.Unauthorized(w, r, mode)
				return
			}
			h.logger.Error().Err(err).Str("agent_id", in.GetId()).Msg("token verify failed")
			response.Unavailable(w, r, mode)
			return
		}
	}

	existing, getErr := h.agentSVC.Get(r.Context(), in.GetId())
	if err = h.agentSVC.Upsert(r.Context(), a); err != nil {
		h.logger.Error().Err(err).Str("agent_id", in.GetId()).Msg("upsert failed")
		response.Unavailable(w, r, mode)
		return
	}
	recordSyncEvents(h.eventHub, in.GetId(), in.GetName(), existing, getErr)

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
	logger      zerolog.Logger
	agentSVC    *agent.Service
	hub         *event.Hub
	requireAuth bool
}

// NewGRPCDiscovery creates a new gRPC discovery handler.
//
// requireAuth turns on per-agent bearer-token authentication (TOFU enroll +
// constant-time verify) on every sync.
func NewGRPCDiscovery(logger zerolog.Logger, agentSVC *agent.Service, hub *event.Hub, requireAuth bool) *GRPCDiscovery {
	if agentSVC == nil {
		panic(service.ErrNilService)
	}
	if hub == nil {
		panic(event.ErrNilHub)
	}
	return &GRPCDiscovery{
		logger:      logger.With().Str("handler", "discovery-grpc").Logger(),
		agentSVC:    agentSVC,
		hub:         hub,
		requireAuth: requireAuth,
	}
}

// Sync implements discoverv1.DiscoverServiceServer.
func (g *GRPCDiscovery) Sync(ctx context.Context, req *discoverv1.SyncRequest) (*discoverv1.SyncResponse, error) {
	a, err := agentFromSyncRequest(req)
	if err != nil {
		return nil, status.Errorf(ctx, codes.InvalidArgument, "invalid agent data: %v", err)
	}

	if g.requireAuth {
		token := grpcBearerToken(ctx)
		if err := g.agentSVC.VerifyOrEnrollToken(ctx, req.GetId(), token); err != nil {
			if errors.Is(err, agent.ErrUnauthenticated) {
				return nil, status.Errorf(ctx, codes.Unauthenticated, "agent authentication failed")
			}
			g.logger.Error().Err(err).Str("agent_id", req.GetId()).Msg("token verify failed")
			return nil, status.FromError(ctx, err).Err()
		}
	}

	existing, getErr := g.agentSVC.Get(ctx, req.GetId())
	if err = g.agentSVC.Upsert(ctx, a); err != nil {
		g.logger.Error().Err(err).Str("agent_id", req.GetId()).Msg("upsert failed")
		return nil, status.FromError(ctx, err).Err()
	}
	recordSyncEvents(g.hub, req.GetId(), req.GetName(), existing, getErr)
	return &discoverv1.SyncResponse{Success: true}, nil
}

// agentFromSyncRequest builds a domain Agent from a discovery heartbeat. Shared
// by the HTTP and gRPC Sync handlers — both carry the same proto SyncRequest.
func agentFromSyncRequest(req *discoverv1.SyncRequest) (*model.Agent, error) {
	return model.NewAgentFrom(model.AgentParams{
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
}

// recordSyncEvents emits the connection / issue-resolution events shared by both
// Sync transports after a successful upsert. existing and getErr describe the
// agent state observed *before* the upsert (getErr == ErrNotFound ⇒ brand new;
// a non-active existing agent ⇒ reconnect that clears outstanding issues).
func recordSyncEvents(hub *event.Hub, id, name string, existing *model.Agent, getErr error) {
	switch {
	case errors.Is(getErr, storage.ErrNotFound):
		hub.Record(event.AgentConnected, event.Payload{ID: id, Name: name, By: "discovery"})
	case existing != nil && existing.Status() != enum.AgentStatusActive:
		hub.Record(event.AgentConnected, event.Payload{ID: id, Name: name, By: "discovery"})

		n := hub.DeleteIssues(event.AgentInactive, id)
		n += hub.DeleteIssues(event.AgentDisconnected, id)
		if n > 0 {
			hub.Record(event.IssueClosed, event.Payload{ID: id, Name: name, By: "discovery"})
			hub.Notify(event.RefreshDashboard)
		}
	}
	hub.Notify(event.RefreshAgents)
}

// bearerToken extracts the credential from an Authorization header value,
// accepting the scheme case-insensitively. Returns "" if absent/malformed.
func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) >= len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return header[len(prefix):]
	}
	return ""
}

// grpcBearerToken extracts the bearer token from incoming gRPC "authorization"
// metadata. Returns "" if absent/malformed.
func grpcBearerToken(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return ""
	}
	return bearerToken(vals[0])
}
