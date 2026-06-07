package status

import (
	"context"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/soltiHQ/control-plane/internal/transport/errkind"
	"github.com/soltiHQ/control-plane/internal/transportctx"
)

// FromError maps a domain error to a gRPC status.
func FromError(ctx context.Context, err error) *grpcstatus.Status {
	if err == nil {
		return grpcstatus.New(codes.OK, "")
	}

	var (
		code, msg = mapError(err)
		st        = grpcstatus.New(code, msg)
	)
	transportctx.SetError(ctx, msg)
	if rid, ok := transportctx.RequestID(ctx); ok {
		if detailed, detailErr := withRequestID(st, rid); detailErr == nil {
			st = detailed
		}
	}
	return st
}

// Errorf creates a gRPC status error with explicit code and message.
func Errorf(ctx context.Context, code codes.Code, format string, args ...any) error {
	st := grpcstatus.Newf(code, format, args...)
	transportctx.SetError(ctx, st.Message())

	if rid, ok := transportctx.RequestID(ctx); ok {
		if detailed, err := withRequestID(st, rid); err == nil {
			st = detailed
		}
	}
	return st.Err()
}

// mapError translates the shared errkind.Kind into a gRPC code and client message.
func mapError(err error) (codes.Code, string) {
	switch errkind.Classify(err) {
	case errkind.Canceled:
		return codes.Canceled, "request canceled"
	case errkind.DeadlineExceeded:
		return codes.DeadlineExceeded, "deadline exceeded"
	case errkind.Unauthenticated:
		return codes.Unauthenticated, "unauthenticated"
	case errkind.PermissionDenied:
		return codes.PermissionDenied, "permission denied"
	case errkind.InvalidArgument:
		return codes.InvalidArgument, "invalid argument"
	case errkind.FailedPrecondition:
		return codes.FailedPrecondition, "user disabled"
	case errkind.NotFound:
		return codes.NotFound, "not found"
	case errkind.AlreadyExists:
		return codes.AlreadyExists, "already exists"
	case errkind.Conflict:
		return codes.Aborted, "conflict"
	case errkind.Unavailable:
		return codes.Unavailable, "unavailable"
	default:
		return codes.Internal, "internal error"
	}
}

// withRequestID attaches a request ID to the gRPC status as RequestInfo detail.
func withRequestID(st *grpcstatus.Status, rid string) (*grpcstatus.Status, error) {
	return st.WithDetails(&errdetails.RequestInfo{
		RequestId: rid,
	})
}
