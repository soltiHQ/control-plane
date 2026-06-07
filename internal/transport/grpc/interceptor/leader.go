package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/soltiHQ/control-plane/internal/cluster"
)

// LeaderOptions tunes the [UnaryLeader] interceptor.
type LeaderOptions struct {
	// IsWrite decides whether an RPC must run on the leader.
	// The argument is the full method name, e.g. "/solti.discover.v1.DiscoverService/Sync".
	// If nil, every call is treated as a writing safe default.
	IsWrite func(fullMethod string) bool
}

// UnaryLeader returns codes.Unavailable with an "x-leader" metadata entry when a write RPC hits a follower.
// Client-side grpc with retry policy (or just a round-robin resolver) will try another backend on Unavailable.
func UnaryLeader(leadership cluster.Leadership, opts LeaderOptions) grpc.UnaryServerInterceptor {
	isWrite := opts.IsWrite
	if isWrite == nil {
		isWrite = allWrites
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !isWrite(info.FullMethod) || leadership.AmLeader() {
			return handler(ctx, req)
		}
		if addr := leadership.CurrentLeader(); addr != "" {
			_ = grpc.SetHeader(ctx, metadata.Pairs("x-leader", addr))
		}
		return nil, status.Error(codes.Unavailable, "not the leader")
	}
}

func allWrites(string) bool { return true }
