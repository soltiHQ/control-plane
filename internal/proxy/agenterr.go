package proxy

import (
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/soltiHQ/control-plane/internal/transport/errkind"
)

// AgentError is the proxy's anti-corruption wrapper for an error returned by an
// agent. It translates the agent's wire status (gRPC code / HTTP status) into a
// transport-agnostic errkind.Kind at the boundary, so the rest of the control
// plane classifies it through the single errkind contract — without sniffing
// gRPC/HTTP details downstream.
type AgentError struct {
	kind errkind.Kind
	err  error
}

func (e *AgentError) Error() string { return e.err.Error() }

func (e *AgentError) Unwrap() error { return e.err }

func (e *AgentError) ErrorKind() errkind.Kind { return e.kind }

// agentError wraps an error from an agent call: it keeps the op sentinel and the
// underlying error in the chain (for errors.Is) and tags the boundary Kind
// derived from the agent's wire status.
func agentError(op, agentErr error) error {
	return &AgentError{
		kind: classifyAgentErr(agentErr),
		err:  fmt.Errorf("%w: %w", op, agentErr),
	}
}

// classifyAgentErr derives an errkind.Kind from an agent's transport error: a
// gRPC status code, or an HTTP status carried by an unexpectedStatusError.
// Anything without a recognizable status (CP-side dial/decode failures) is
// Internal.
func classifyAgentErr(err error) errkind.Kind {
	if st, ok := status.FromError(err); ok && st.Code() != codes.OK {
		return fromGRPCCode(st.Code())
	}
	var hs interface{ HTTPStatus() int }
	if errors.As(err, &hs) {
		return fromHTTPStatus(hs.HTTPStatus())
	}
	return errkind.Internal
}

func fromGRPCCode(c codes.Code) errkind.Kind {
	switch c {
	case codes.InvalidArgument:
		return errkind.InvalidArgument
	case codes.Unauthenticated:
		return errkind.Unauthenticated
	case codes.PermissionDenied:
		return errkind.PermissionDenied
	case codes.NotFound:
		return errkind.NotFound
	case codes.AlreadyExists:
		return errkind.AlreadyExists
	case codes.Aborted:
		return errkind.Conflict
	case codes.FailedPrecondition:
		return errkind.FailedPrecondition
	case codes.Unavailable, codes.ResourceExhausted:
		return errkind.Unavailable
	case codes.Canceled:
		return errkind.Canceled
	case codes.DeadlineExceeded:
		return errkind.DeadlineExceeded
	default:
		return errkind.Internal
	}
}

func fromHTTPStatus(code int) errkind.Kind {
	switch code {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return errkind.InvalidArgument
	case http.StatusUnauthorized:
		return errkind.Unauthenticated
	case http.StatusForbidden:
		return errkind.PermissionDenied
	case http.StatusNotFound:
		return errkind.NotFound
	case http.StatusConflict:
		return errkind.Conflict
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return errkind.Unavailable
	default:
		return errkind.Internal
	}
}
